// Package backup 提供一键备份/恢复：DB 全表 JSON + DATA_DIR 目录打包为 tar.gz。
// 可选用站内主密钥对整包做流式 AES-256-GCM 加密（包内含服务商凭证等敏感信息）。
package backup

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// magic 是加密备份文件头（Sweetcorn Backup v1）。明文备份是 gzip（1f 8b），不会命中。
const magic = "SCB1"

// chunkSize 为流式加密分块大小。
const chunkSize = 1 << 20

// Entry 描述一个备份文件。
type Entry struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	SHA256    string    `json:"sha256"`
	Encrypted bool      `json:"encrypted"`
}

// Create 导出所有表为 db.json，并连同 dataDir 打包到 out。
// master 非空时对整包加密（流式 AES-256-GCM）。
func Create(db *gorm.DB, dataDir, out string, master []byte) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var sink io.Writer = f
	var enc *encWriter
	if len(master) > 0 {
		enc, err = newEncWriter(f, master)
		if err != nil {
			return err
		}
		sink = enc
	}
	gz := gzip.NewWriter(sink)
	tw := tar.NewWriter(gz)

	data, err := exportDB(db)
	if err != nil {
		return err
	}
	blob, _ := json.Marshal(data)
	if err := writeTar(tw, "db.json", blob); err != nil {
		return err
	}
	// DATA_DIR（证书、blobs、zones.json 等）
	if dataDir != "" {
		base := filepath.Dir(dataDir)
		_ = filepath.Walk(dataDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			return writeTar(tw, filepath.ToSlash(rel), b)
		})
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if enc != nil {
		return enc.Close()
	}
	return nil
}

// Restore 从备份包恢复：先自动把当前状态备份到 <in>.pre-restore-<时间>，
// 再建表（调用方应已 AutoMigrate）、逐表插入，最后解包 DATA_DIR。
// 返回恢复前自动备份的路径，便于失败回滚。明文/加密包自动识别。
func Restore(db *gorm.DB, dataDir, in string, master []byte) (string, error) {
	safety := in + ".pre-restore-" + time.Now().Format("20060102-150405")
	if err := Create(db, dataDir, safety, master); err != nil {
		return "", fmt.Errorf("恢复前自动备份失败: %w", err)
	}

	f, err := os.Open(in)
	if err != nil {
		return safety, err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 1<<20)
	head, err := br.Peek(4)
	if err != nil {
		return safety, err
	}
	var src io.Reader = br
	if string(head) == magic {
		if _, err := io.ReadFull(br, make([]byte, 4)); err != nil {
			return safety, err
		}
		dr, err := newDecReader(br, master)
		if err != nil {
			return safety, err
		}
		src = dr
	}
	gz, err := gzip.NewReader(src)
	if err != nil {
		return safety, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return safety, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(tr, 1<<30))
		if h.Name == "db.json" {
			var data map[string][]map[string]any
			if err := json.Unmarshal(b, &data); err != nil {
				return safety, err
			}
			if err := importDB(db, data); err != nil {
				return safety, err
			}
			continue
		}
		// DATA_DIR 文件（拒绝路径穿越）
		clean := filepath.Clean(filepath.FromSlash(h.Name))
		if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, "..") {
			continue
		}
		dest := filepath.Join(filepath.Dir(dataDir), clean)
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.WriteFile(dest, b, 0o600)
	}
	return safety, nil
}

func exportDB(db *gorm.DB) (map[string][]map[string]any, error) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return nil, err
	}
	out := map[string][]map[string]any{}
	for _, t := range tables {
		var rows []map[string]any
		if err := db.Table(t).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("导出 %s: %w", t, err)
		}
		out[t] = rows
	}
	return out, nil
}

func importDB(db *gorm.DB, data map[string][]map[string]any) error {
	for t, rows := range data {
		if len(rows) == 0 {
			continue
		}
		if err := db.Table(t).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("导入 %s: %w", t, err)
		}
	}
	return nil
}

// Scheduled 生成一个带时间戳的备份并清理旧份数。
func Scheduled(db *gorm.DB, dataDir, dir string, keep int, master []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, "mailserver-"+time.Now().Format("20060102-150405")+".tar.gz")
	if err := Create(db, dataDir, out, master); err != nil {
		return "", err
	}
	Prune(dir, keep)
	return out, nil
}

// Prune 仅保留最新的 keep 个备份。
func Prune(dir string, keep int) {
	if keep <= 0 {
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "mailserver-*.tar.gz"))
	sort.Strings(files)
	for len(files) > keep {
		_ = os.Remove(files[0])
		files = files[1:]
	}
}

// List 返回目录下的备份（按时间倒序，最新在前），并计算 sha256 与是否加密。
func List(dir string) ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "mailserver-*.tar.gz"))
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(files))
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		e := Entry{Name: filepath.Base(p), Size: info.Size(), ModTime: info.ModTime()}
		if sum, err := Checksum(p); err == nil {
			e.SHA256 = sum
		}
		e.Encrypted = IsEncrypted(p)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Checksum 计算文件 sha256（十六进制）。
func Checksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsEncrypted 判断备份是否为加密格式。
func IsEncrypted(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return false
	}
	return string(head[:]) == magic
}

// SafeName 校验备份文件名，防止路径穿越。
func SafeName(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return false
	}
	return strings.HasPrefix(name, "mailserver-") && strings.HasSuffix(name, ".tar.gz")
}

// ---- 流式加密：分块 AES-256-GCM，密钥 = HMAC-SHA256(master, 随机 salt) ----

func deriveKey(master, salt []byte) []byte {
	h := hmac.New(sha256.New, master)
	h.Write(salt)
	return h.Sum(nil)
}

func chunkNonce(base []byte, ctr uint32) []byte {
	nonce := make([]byte, 12)
	copy(nonce, base)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], ctr)
	for i := 0; i < 4; i++ {
		nonce[8+i] ^= b[i]
	}
	return nonce
}

type encWriter struct {
	w    io.Writer
	gcm  cipher.AEAD
	base []byte
	buf  []byte
	ctr  uint32
	err  error
}

func newEncWriter(w io.Writer, master []byte) (*encWriter, error) {
	salt := make([]byte, 16)
	base := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(base); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(deriveKey(master, salt))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(magic)); err != nil {
		return nil, err
	}
	if _, err := w.Write(salt); err != nil {
		return nil, err
	}
	if _, err := w.Write(base); err != nil {
		return nil, err
	}
	return &encWriter{w: w, gcm: gcm, base: base, buf: make([]byte, 0, chunkSize)}, nil
}

func (e *encWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	total := len(p)
	for len(p) > 0 {
		space := chunkSize - len(e.buf)
		n := len(p)
		if n > space {
			n = space
		}
		e.buf = append(e.buf, p[:n]...)
		p = p[n:]
		if len(e.buf) == chunkSize {
			if err := e.flush(); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}

func (e *encWriter) flush() error {
	if len(e.buf) == 0 {
		return nil
	}
	ct := e.gcm.Seal(nil, chunkNonce(e.base, e.ctr), e.buf, nil)
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(ct)))
	if _, err := e.w.Write(lb[:]); err != nil {
		e.err = err
		return err
	}
	if _, err := e.w.Write(ct); err != nil {
		e.err = err
		return err
	}
	e.ctr++
	e.buf = e.buf[:0]
	return nil
}

func (e *encWriter) Close() error { return e.flush() }

type decReader struct {
	r    io.Reader
	gcm  cipher.AEAD
	base []byte
	ctr  uint32
	buf  []byte
	err  error
}

func newDecReader(r io.Reader, master []byte) (*decReader, error) {
	salt := make([]byte, 16)
	base := make([]byte, 12)
	if _, err := io.ReadFull(r, salt); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, base); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(deriveKey(master, salt))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &decReader{r: r, gcm: gcm, base: base}, nil
}

func (d *decReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		if d.err != nil {
			return 0, d.err
		}
		var lb [4]byte
		if _, err := io.ReadFull(d.r, lb[:]); err != nil {
			if err == io.EOF {
				return 0, io.EOF
			}
			return 0, err
		}
		n := binary.BigEndian.Uint32(lb[:])
		if n == 0 || n > chunkSize+64 {
			return 0, errors.New("backup: 加密分块损坏")
		}
		ct := make([]byte, n)
		if _, err := io.ReadFull(d.r, ct); err != nil {
			return 0, err
		}
		pt, err := d.gcm.Open(nil, chunkNonce(d.base, d.ctr), ct, nil)
		if err != nil {
			return 0, fmt.Errorf("backup: 解密失败（主密钥不匹配？）: %w", err)
		}
		d.ctr++
		d.buf = pt
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func writeTar(tw *tar.Writer, name string, b []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b))}); err != nil {
		return err
	}
	_, err := tw.Write(b)
	return err
}
