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
