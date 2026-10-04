// Package blob 提供内容寻址的附件存储：DATA_DIR/blobs/{sha256}。
// 相同内容天然去重；DB 只需存 blob_id + 元数据，避免 base64 塞进 PG text 的三重膨胀。
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var idRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Store struct{ dir string }

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Put 写入内容并返回 sha256 十六进制 id；已存在则直接复用（去重）。
func (s *Store) Put(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])
	p := s.path(id)
	if _, err := os.Stat(p); err == nil {
		return id, nil
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return id, nil
}

// Get 读取内容；id 非法或不存在返回错误。
func (s *Store) Get(id string) ([]byte, error) {
	if !idRe.MatchString(id) {
		return nil, errors.New("blob: bad id")
	}
	return os.ReadFile(s.path(id))
}

// Delete 删除内容（不存在视为成功）。
func (s *Store) Delete(id string) error {
	if !idRe.MatchString(id) {
		return errors.New("blob: bad id")
	}
	if err := os.Remove(s.path(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id) }

// List 返回全部 blob id（合法 sha256 文件）。
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && idRe.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
