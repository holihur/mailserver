package backup

import (
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
	g.Exec("TRUNCATE users, mails, mail_aliases, mail_rules, contacts, mail_tokens, domains, dns_records, settings, audit_logs, login_events, scheduled_mails, sieve_scripts, mail_folders, mail_routes, external_accounts, dns_providers, acme_configs RESTART IDENTITY CASCADE")
	if err := g.Create(&model.User{Email: "bk@test.local", Name: "bk"}).Error; err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bk.tar.gz")
	if err := Create(g, "", out); err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	g.Exec("TRUNCATE users RESTART IDENTITY CASCADE")
	if err := Restore(g, "", out); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	var n int64
	g.Model(&model.User{}).Where("email = ?", "bk@test.local").Count(&n)
	if n != 1 {
		t.Fatalf("恢复后应有 1 个用户，得到 %d", n)
	}
}
