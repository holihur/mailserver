package certstore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func selfSigned(t *testing.T, dns string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dns},
		DNSNames:     []string{dns},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return
}

func TestSetAndMeta(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	certPEM, keyPEM := selfSigned(t, "mail.example.com")
	if err := s.Set(certPEM, keyPEM, "manual"); err != nil {
		t.Fatal(err)
	}
	m := s.Meta()
	if !m.Exists || m.Source != "manual" {
		t.Fatalf("unexpected meta: %+v", m)
	}
	if len(m.Domains) != 1 || m.Domains[0] != "mail.example.com" {
		t.Fatalf("unexpected domains: %+v", m.Domains)
	}
	if _, err := s.GetCertificate(nil); err != nil {
		t.Fatalf("get certificate: %v", err)
	}

	// 新 store 从磁盘恢复
	s2 := New(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if !s2.Meta().Exists {
		t.Fatal("reload should have cert")
	}
}

func TestClear(t *testing.T) {
	s := New(t.TempDir())
	certPEM, keyPEM := selfSigned(t, "mail.example.com")
	if err := s.Set(certPEM, keyPEM, "acme"); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if s.Meta().Exists {
		t.Fatal("cert should be cleared")
	}
	if _, err := s.GetCertificate(nil); err == nil {
		t.Fatal("expected error when no cert")
	}
}

func TestBadPEM(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set([]byte("not a cert"), []byte("not a key"), "manual"); err == nil {
		t.Fatal("expected error for bad PEM")
	}
}

func TestLoadEmptyAndTLSConfig(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if s.Meta().Exists {
		t.Fatal("should not exist")
	}
	if s.TLSConfig() == nil || s.TLSConfig().GetCertificate == nil {
		t.Fatal("tls config should provide GetCertificate")
	}
}
