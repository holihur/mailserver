// Package selfupdate 实现 `mailserver update`：从 GitHub Release 拉取最新单二进制，
// 校验 sha256 后原子替换当前可执行文件，并尝试重启 systemd 服务。
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultRepo = "holihur/mailserver"
	binName     = "mailserver"
)

// Options 控制更新行为。
type Options struct {
	Repo    string // GitHub owner/repo
	Current string // 当前版本（可含 v，可为 dev）
	Force   bool   // 即使已是最新也重装
	Log     func(string, ...any)
}

func (o *Options) log(format string, args ...any) {
	if o.Log != nil {
		o.Log(format, args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Latest 查询 GitHub 最新 release tag（供管理后台「检查更新」使用）。
func Latest(ctx context.Context, repo string) (string, error) {
	if repo == "" {
		repo = DefaultRepo
	}
	return latestTag(ctx, &http.Client{Timeout: 20 * time.Second}, repo)
}

// Differs 判断两个版本号是否不同（忽略前缀 v 与首尾空白；latest 为空视为相同）。
func Differs(current, latest string) bool {
	l := normVer(latest)
	return l != "" && normVer(current) != l
}

// Run 执行更新。
func Run(ctx context.Context, o Options) error {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	client := &http.Client{Timeout: 5 * time.Minute}

	tag, err := latestTag(ctx, client, o.Repo)
	if err != nil {
		return err
	}
	if !o.Force && normVer(o.Current) == normVer(tag) && normVer(tag) != "" {
		o.log("已是最新版本 %s", tag)
		return nil
	}
	o.log("发现版本 %s（当前 %s），开始下载…", tag, o.Current)

	asset := AssetName(tag, runtime.GOARCH)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s/", o.Repo, tag)

	sums, err := httpGet(ctx, client, base+"checksums.txt")
	if err != nil {
		return fmt.Errorf("下载校验和失败: %w", err)
	}
	want, err := ParseChecksums(string(sums), asset)
	if err != nil {
		return err
	}
	data, err := httpGet(ctx, client, base+asset)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %w", asset, err)
	}
	// checksums.txt 是 GoReleaser 对归档(.tar.gz)算的 sha256，校验归档本身
	if got := sha256Hex(data); !strings.EqualFold(got, want) {
		return fmt.Errorf("校验失败：期望 %s，实际 %s", want, got)
	}
	o.log("校验通过（sha256 %s）", want[:12])
	bin, err := ExtractBinary(data, binName)
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if err := replaceBinary(exe, bin); err != nil {
		return err
	}
	o.log("已更新到 %s：%s", tag, exe)
	restartService(o)
	return nil
}

// AssetName 返回发布资产名（与 GoReleaser name_template 一致）。
func AssetName(tag, arch string) string {
	return fmt.Sprintf("mailserver_%s_linux_%s.tar.gz", tag, arch)
}

// ParseChecksums 从 sha256sum 文本里取出指定文件的哈希。
func ParseChecksums(text, file string) (string, error) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if path.Base(strings.TrimPrefix(f[1], "*")) == file || path.Base(f[1]) == file {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("校验和里找不到 %s", file)
}

// ExtractBinary 从 tar.gz 中取出指定名字的文件。
func ExtractBinary(tarGz []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarGz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if path.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
	return nil, fmt.Errorf("压缩包中没有 %s", name)
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func replaceBinary(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".mailserver-new-*")
	if err != nil {
		return fmt.Errorf("无法写入 %s（需要 root？）: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// 备份旧文件，再原子替换
	_ = os.Remove(exe + ".bak")
	if err := os.Rename(exe, exe+".bak"); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(exe+".bak", exe)
		os.Remove(tmpName)
		return err
	}
	return nil
}

func restartService(o Options) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		o.log("请手动重启服务以生效")
		return
	}
	if out, err := exec.Command("systemctl", "restart", "mailserver").CombinedOutput(); err != nil {
		o.log("自动重启失败（%s），请手动执行：systemctl restart mailserver", strings.TrimSpace(string(out)))
		return
	}
	o.log("已重启 mailserver.service")
}

func normVer(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func latestTag(ctx context.Context, client *http.Client, repo string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mailserver-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("查询最新版本失败：HTTP %d", resp.StatusCode)
	}
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", errors.New("未能解析最新版本号")
	}
	return r.TagName, nil
}

func httpGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "mailserver-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}
