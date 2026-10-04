package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	if got := AssetName("v0.3.2", "amd64"); got != "mailserver_v0.3.2_linux_amd64.tar.gz" {
		t.Fatalf("AssetName=%q", got)
	}
}

func TestNormVer(t *testing.T) {
	if normVer(" v0.3.2 ") != "0.3.2" || normVer("dev") != "dev" {
		t.Fatal("normVer failed")
	}
}

func TestParseChecksums(t *testing.T) {
	text := "abc123  mailserver_v1_linux_amd64.tar.gz\ndef456  ./nsd_v1_linux_amd64.tar.gz\n"
	if got, err := ParseChecksums(text, "mailserver_v1_linux_amd64.tar.gz"); err != nil || got != "abc123" {
		t.Fatalf("got %q err %v", got, err)
	}
	if got, err := ParseChecksums(text, "nsd_v1_linux_amd64.tar.gz"); err != nil || got != "def456" {
		t.Fatalf("dot-slash parse: got %q err %v", got, err)
	}
	if _, err := ParseChecksums(text, "missing.tar.gz"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func tarGzWith(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	data := tarGzWith(t, "mailserver", []byte("BINARY"))
	got, err := ExtractBinary(data, "mailserver")
	if err != nil || string(got) != "BINARY" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := ExtractBinary(data, "nope"); err == nil {
		t.Fatal("expected error for missing entry")
	}
	if _, err := ExtractBinary([]byte("not gzip"), "mailserver"); err == nil {
		t.Fatal("expected error for bad gzip")
	}
}

func TestChecksumOverArchive(t *testing.T) {
	// checksums.txt 是对归档(.tar.gz)的 sha256，不是解出的二进制
	data := tarGzWith(t, "mailserver", []byte("BIN"))
	sum := sha256Hex(data)
	asset := AssetName("v1.0.0", "amd64")
	got, err := ParseChecksums(sum+"  "+asset+"\n", asset)
	if err != nil || got != sum {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestShaAndReplace(t *testing.T) {
	if len(sha256Hex([]byte("x"))) != 64 {
		t.Fatal("sha256 length")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "mailserver")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(exe, []byte("NEW")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "NEW" {
		t.Fatalf("got %q", got)
	}
	bak, _ := os.ReadFile(exe + ".bak")
	if string(bak) != "OLD" {
		t.Fatalf("backup got %q", bak)
	}
	// 信息权限应为可执行
	if fi, err := os.Stat(exe); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("not executable: %v", fi.Mode())
	}
}

func TestDiffers(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.3.14", "v0.3.14", false},
		{"0.3.14", "v0.3.14", false},
		{"v0.3.14", "v0.3.15", true},
		{"dev", "v0.3.15", true},
		{"v0.3.14", "", false},
	}
	for _, c := range cases {
		if got := Differs(c.cur, c.latest); got != c.want {
			t.Errorf("Differs(%q,%q)=%v want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestRepoAllowed(t *testing.T) {
	os.Unsetenv("MAILSERVER_REPO_ALLOW_ANY")
	if !RepoAllowed("holihur/mailserver") {
		t.Fatal("官方仓库应允许")
	}
	if !RepoAllowed("Holihur/mailserver") {
		t.Fatal("大小写不敏感")
	}
	if RepoAllowed("evil/mailserver") {
		t.Fatal("未知仓库应拒绝")
	}
	t.Setenv("MAILSERVER_REPO_ALLOW_ANY", "1")
	if !RepoAllowed("evil/mailserver") {
		t.Fatal("放开后应允许")
	}
}
