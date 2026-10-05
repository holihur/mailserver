package backup

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #13：备份 -> 清空 -> 恢复 后数据仍在。
func TestBackupRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails, mail_aliases, mail_rules, contacts, mail_tokens, domains, dns_records, settings, audit_logs, login_events, scheduled_mails, sieve_scripts, mail_folders, mail_routes, external_accounts, dns_providers, acme_configs, sessions RESTART IDENTITY CASCADE")
	if err := g.Create(&model.User{Email: "bk@test.local", Name: "bk"}).Error; err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bk.tar.gz")
	if err := Create(g, "", out, nil); err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	g.Exec("TRUNCATE users, sessions RESTART IDENTITY CASCADE")
	if _, err := Restore(g, "", out, nil); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	var n int64
	g.Model(&model.User{}).Where("email = ?", "bk@test.local").Count(&n)
	if n != 1 {
		t.Fatalf("恢复后应有 1 个用户，得到 %d", n)
	}
}

// #13：整包流式加密可往返；错误主密钥必须解密失败。
func TestEncryptRoundTrip(t *testing.T) {
	plain := bytes.Repeat([]byte("sweetcorn-backup-"), 200000) // ~3.4MB，跨多个分块
	var enc bytes.Buffer
	ew, err := newEncWriter(&enc, []byte("master-key-1234567890"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ew.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := ew.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(enc.Bytes(), []byte(magic)) {
		t.Fatalf("应带加密魔数")
	}
	if bytes.Contains(enc.Bytes(), []byte("sweetcorn-backup-")) {
		t.Fatalf("密文不应包含明文")
	}

	r := bytes.NewReader(append([]byte(magic), enc.Bytes()[len(magic):]...))
	if _, err := io.ReadFull(r, make([]byte, 4)); err != nil {
		t.Fatal(err)
	}
	dr, err := newDecReader(r, []byte("master-key-1234567890"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(dr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("解密结果不一致: %d vs %d", len(got), len(plain))
	}

	// 错误主密钥
	r2 := bytes.NewReader(append([]byte(magic), enc.Bytes()[len(magic):]...))
	_, _ = io.ReadFull(r2, make([]byte, 4))
	dr2, err := newDecReader(r2, []byte("wrong-master-key-000000"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(dr2); err == nil {
		t.Fatalf("错误主密钥应解密失败")
	}
}
