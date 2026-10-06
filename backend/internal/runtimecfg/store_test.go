package runtimecfg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mailserver/internal/config"
	"mailserver/internal/dkim"
	"mailserver/internal/model"
	"mailserver/internal/secret"
)

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"Example.COM.":        "example.com",
		"  mail.example.com ": "mail.example.com",
		"...":                 "",
		"":                    "",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStoreInMemory(t *testing.T) {
	secret.SetKey("unit-key")
	s := &Store{vals: map[string]string{}}

	s.vals[KeyMailHost] = "mail.example.com"
	if s.MailHost() != "mail.example.com" {
		t.Fatalf("MailHost=%q", s.MailHost())
	}
	// nil db：仅内存生效，不应 panic
	s.Set(KeyPublicIP, "1.2.3.4")
	s.SetMany(map[string]string{KeyAdminEmails: "a@b.c", KeyRelayPort: "587"})
	if s.Get(KeyPublicIP) != "1.2.3.4" || s.PublicIP() != "1.2.3.4" {
		t.Fatalf("PublicIP=%q", s.PublicIP())
	}
	if s.AdminEmails() != "a@b.c" {
		t.Fatalf("AdminEmails=%q", s.AdminEmails())
	}
	r := s.Relay()
	if r.Name != "mail.example.com" || r.Port != "587" {
		t.Fatalf("relay=%+v", r)
	}
	snap := s.Snapshot()
	if snap["mail_host"] != "mail.example.com" || snap["dkim_ready"] != false {
		t.Fatalf("snapshot=%+v", snap)
	}
}

func TestStoreAutoUpdate(t *testing.T) {
	s := &Store{vals: map[string]string{}}
	// 默认：关闭自动更新，间隔 10 分钟
	if s.AutoUpdate() {
		t.Fatal("默认应关闭自动更新")
	}
	if got := s.UpdateInterval(); got != 10*time.Minute {
		t.Fatalf("默认间隔应 10 分钟，得到 %v", got)
	}
	// 开启 + 自定义间隔
	s.vals[KeyAutoUpdate] = "on"
	s.vals[KeyUpdateInterval] = "30"
	if !s.AutoUpdate() || s.UpdateInterval() != 30*time.Minute {
		t.Fatalf("auto=%v interval=%v", s.AutoUpdate(), s.UpdateInterval())
	}
	// 非法/越界值回退到合法范围
	for _, in := range []string{"0", "-5", "abc", "99999"} {
		s.vals[KeyUpdateInterval] = in
		if got := s.UpdateInterval(); got <= 0 || got > 24*time.Hour {
			t.Fatalf("interval(%q)=%v 应回退到 1~1440 分钟", in, got)
		}
	}
	s.vals[KeyUpdateInterval] = "1440"
	if s.UpdateInterval() != 24*time.Hour {
		t.Fatal("上限应为 1440 分钟")
	}
	// Snapshot 暴露字段
	s.vals[KeyAutoUpdate] = "1"
	s.vals[KeyUpdateInterval] = "15"
	snap := s.Snapshot()
	if snap["auto_update"] != true || snap["update_interval"] != 15 {
		t.Fatalf("snapshot auto=%v interval=%v", snap["auto_update"], snap["update_interval"])
	}
}

func TestNewStoreMerge(t *testing.T) {
	secret.SetKey("k")
	cfg := config.Config{
		Host: "mail.example.com", AdminEmails: "a@b.c",
		RelayHost: "relay", RelayPort: "587", DKIMDomain: "example.com", DKIMSelector: "dkim",
	}
	rows := []model.Setting{{Key: KeyMailHost, Value: "custom.example.com"}}
	s := newStore(cfg, rows)
	if s.MailHost() != "custom.example.com" {
		t.Fatalf("db should override env, got %q", s.MailHost())
	}
	if s.AdminEmails() != "a@b.c" || s.Get(KeyRelayHost) != "relay" {
		t.Fatalf("env fallback missing: %q %q", s.AdminEmails(), s.Get(KeyRelayHost))
	}
}

func TestNewStoreDKIMFile(t *testing.T) {
	secret.SetKey("k")
	dir := t.TempDir()
	pemBytes, _ := dkim.GeneratePEM()
	path := filepath.Join(dir, "dkim.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Host: "mail.example.com", DKIMKey: path, DKIMDomain: "example.com", DKIMSelector: "dkim"}
	s := newStore(cfg, nil)
	if !s.DKIMReady() {
		t.Fatal("DKIM should be bootstrapped from cfg.DKIMKey file")
	}
}

func TestNewNilDB(t *testing.T) {
	secret.SetKey("k")
	s := New(nil, config.Config{Host: "mail.example.com", RelayPort: "587"})
	if s.MailHost() != "mail.example.com" || s.Relay().Port != "587" {
		t.Fatalf("New(nil) failed: %q %+v", s.MailHost(), s.Relay())
	}
}

func TestStoreDKIM(t *testing.T) {
	secret.SetKey("unit-key")
	s := &Store{vals: map[string]string{}}
	if s.DKIMReady() {
		t.Fatal("should not be ready initially")
	}
	pemBytes, err := dkim.GeneratePEM()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetDKIM("Example.COM", "", pemBytes); err != nil {
		t.Fatal(err)
	}
	if !s.DKIMReady() || s.Signer() == nil {
		t.Fatal("should be ready after SetDKIM")
	}
	if s.Signer().Selector != "dkim" || s.Signer().Domain != "example.com" {
		t.Fatalf("signer=%+v", s.Signer())
	}
	if err := s.SetDKIM("example.com", "dkim", []byte("bad pem")); err == nil {
		t.Fatal("bad key should error")
	}

	// 清空 key 后 Reload 应清空签名器
	s.mu.Lock()
	s.vals[KeyDKIMKeyEnc] = ""
	s.mu.Unlock()
	if err := s.ReloadDKIM(); err != nil || s.Signer() != nil {
		t.Fatalf("reload empty: err=%v signer=%v", err, s.Signer())
	}

	// 损坏密文应报错并清空
	s.mu.Lock()
	s.vals[KeyDKIMKeyEnc] = "!!!not-base64!!!"
	s.vals[KeyDKIMDomain] = "example.com"
	s.mu.Unlock()
	if err := s.ReloadDKIM(); err == nil {
		t.Fatal("corrupt ciphertext should error")
	}
	if s.Signer() != nil {
		t.Fatal("signer should be cleared on error")
	}
}

func TestStoreOIDC(t *testing.T) {
	s := &Store{vals: map[string]string{}}
	if s.OIDCEnabled() {
		t.Fatal("默认应关闭 OIDC")
	}
	s.vals[KeyOIDCEnabled] = "1"
	s.vals[KeyOIDCIssuer] = "https://id.example"
	s.vals[KeyOIDCClientID] = "cid"
	s.vals[KeyOIDCClientSecret] = "sec"
	s.vals[KeyOIDCAutoCreate] = "on"
	if !s.OIDCEnabled() || s.OIDCIssuer() != "https://id.example" || s.OIDCClientID() != "cid" ||
		s.OIDCClientSecret() != "sec" || !s.OIDCAutoCreate() {
		t.Fatalf("oidc 读取异常")
	}
	snap := s.Snapshot()
	if snap["oidc_enabled"] != true || snap["oidc_issuer"] != "https://id.example" ||
		snap["oidc_client_secret_set"] != true || snap["oidc_auto_create"] != true {
		t.Fatalf("snapshot: %+v", snap)
	}
}

func TestStoreBackupAndRegistration(t *testing.T) {
	s := &Store{vals: map[string]string{}}

	// 默认：不开放注册，备份目录空、间隔 24h、保留 7 份
	if s.RegistrationEnabled() {
		t.Fatal("默认应关闭注册")
	}
	if s.BackupDir() != "" {
		t.Fatalf("默认备份目录应为空，得到 %q", s.BackupDir())
	}
	if s.BackupIntervalHours() != 24*time.Hour {
		t.Fatalf("默认间隔应 24h，得到 %v", s.BackupIntervalHours())
	}
	if s.BackupKeep() != 7 {
		t.Fatalf("默认保留应 7，得到 %d", s.BackupKeep())
	}

	s.vals[KeyRegistration] = "on"
	s.vals[KeyBackupDir] = "  /data/backups  "
	s.vals[KeyBackupInterval] = "6"
	s.vals[KeyBackupKeep] = "14"
	if !s.RegistrationEnabled() || s.BackupDir() != "/data/backups" ||
		s.BackupIntervalHours() != 6*time.Hour || s.BackupKeep() != 14 {
		t.Fatalf("自定义读取异常: reg=%v dir=%q int=%v keep=%d",
			s.RegistrationEnabled(), s.BackupDir(), s.BackupIntervalHours(), s.BackupKeep())
	}
	s.vals[KeyBackupDir] = "/data/backups"

	// 非法/越界：间隔回退默认，keep 夹到上限
	s.vals[KeyBackupInterval] = "abc"
	if s.BackupIntervalHours() != 24*time.Hour {
		t.Fatalf("非法间隔应回退 24h，得到 %v", s.BackupIntervalHours())
	}
	s.vals[KeyBackupInterval] = "99999"
	if s.BackupIntervalHours() != 8760*time.Hour {
		t.Fatalf("间隔上限应为 8760h，得到 %v", s.BackupIntervalHours())
	}
	s.vals[KeyBackupKeep] = "99999"
	if s.BackupKeep() != 3650 {
		t.Fatalf("keep 上限应为 3650，得到 %d", s.BackupKeep())
	}

	snap := s.Snapshot()
	if snap["backup_dir"] != "/data/backups" || snap["backup_interval_hours"] != 8760 ||
		snap["backup_keep"] != 3650 || snap["registration_enabled"] != true {
		t.Fatalf("snapshot 备份字段异常: %+v", snap)
	}
}

func TestNewStoreBackupDefaults(t *testing.T) {
	cfg := config.Config{Host: "mail.example.com", BackupDir: "/srv/bk", BackupInterval: 12, BackupKeep: 3}
	s := newStore(cfg, nil)
	if s.BackupDir() != "/srv/bk" || s.BackupIntervalHours() != 12*time.Hour || s.BackupKeep() != 3 {
		t.Fatalf("环境变量默认未生效: dir=%q int=%v keep=%d", s.BackupDir(), s.BackupIntervalHours(), s.BackupKeep())
	}
	// 非法环境默认值回退
	cfg2 := config.Config{Host: "h", BackupInterval: 0, BackupKeep: -1}
	s2 := newStore(cfg2, nil)
	if s2.BackupIntervalHours() != 24*time.Hour || s2.BackupKeep() != 7 {
		t.Fatalf("非法默认未回退: int=%v keep=%d", s2.BackupIntervalHours(), s2.BackupKeep())
	}
}

func TestStoreRetention(t *testing.T) {
	s := &Store{vals: map[string]string{}}
	if s.LoginRetentionDays() != 90 || s.AuditRetentionDays() != 180 {
		t.Fatalf("默认应为 90/180，得到 %d/%d", s.LoginRetentionDays(), s.AuditRetentionDays())
	}
	s.vals[KeyLoginRetentionDays] = "30"
	s.vals[KeyAuditRetentionDays] = "365"
	if s.LoginRetentionDays() != 30 || s.AuditRetentionDays() != 365 {
		t.Fatalf("自定义读取异常: %d/%d", s.LoginRetentionDays(), s.AuditRetentionDays())
	}
	s.vals[KeyLoginRetentionDays] = "99999"
	if s.LoginRetentionDays() != 3650 {
		t.Fatalf("上限应为 3650，得到 %d", s.LoginRetentionDays())
	}
	snap := s.Snapshot()
	if snap["login_retention_days"] != 3650 || snap["audit_retention_days"] != 365 {
		t.Fatalf("snapshot: %+v", snap)
	}
}
