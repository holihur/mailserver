package config

import "testing"

func TestDkimDomain(t *testing.T) {
	cases := []struct{ explicit, host, want string }{
		{"example.com", "mail.other.com", "example.com"},
		{"", "mail.example.com", "example.com"},
		{"", "example.com", "example.com"},
		{"", "localhost", "localhost"},
		{"", "MAIL.Example.COM.", "example.com"},
	}
	for _, c := range cases {
		if got := dkimDomain(c.explicit, c.host); got != c.want {
			t.Errorf("dkimDomain(%q,%q)=%q want %q", c.explicit, c.host, got, c.want)
		}
	}
}

func TestGetenv(t *testing.T) {
	t.Setenv("MS_TEST_KEY", "")
	if getenv("MS_TEST_KEY", "def") != "def" {
		t.Fatal("expected default")
	}
	t.Setenv("MS_TEST_KEY", "val")
	if getenv("MS_TEST_KEY", "def") != "val" {
		t.Fatal("expected value")
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/m?sslmode=disable")
	t.Setenv("DATA_DIR", "/data")
	t.Setenv("CERT_DIR", "")
	t.Setenv("MAIL_HOST", "mail.example.com")
	t.Setenv("ADMIN_EMAILS", "a@b.c")
	t.Setenv("SMTP_RELAY_HOST", "relay.example.com")
	t.Setenv("SUBMIT_TLS_PORT", "465")

	cfg := Load()
	if cfg.Port != "9090" {
		t.Errorf("Port=%q", cfg.Port)
	}
	if cfg.DatabaseURL == "" || cfg.DataDir != "/data" {
		t.Errorf("db/data: %q %q", cfg.DatabaseURL, cfg.DataDir)
	}
	if cfg.CertDir != "/data/certs" {
		t.Errorf("CertDir=%q", cfg.CertDir)
	}
	if cfg.Host != "mail.example.com" || cfg.AdminEmails != "a@b.c" {
		t.Errorf("host/admin: %q %q", cfg.Host, cfg.AdminEmails)
	}
	if cfg.DKIMDomain != "example.com" {
		t.Errorf("DKIMDomain=%q", cfg.DKIMDomain)
	}
	if cfg.SubmitTLSPort != "465" || cfg.RelayHost != "relay.example.com" {
		t.Errorf("ports/relay: %q %q", cfg.SubmitTLSPort, cfg.RelayHost)
	}
}

func TestLoadCertDirExplicit(t *testing.T) {
	t.Setenv("DATA_DIR", "/srv/mail")
	t.Setenv("CERT_DIR", "/custom/certs")
	cfg := Load()
	if cfg.CertDir != "/custom/certs" {
		t.Errorf("CertDir=%q", cfg.CertDir)
	}
}
