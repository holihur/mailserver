package dkim

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateLoadSign(t *testing.T) {
	pemBytes, err := GeneratePEM()
	if err != nil {
		t.Fatal(err)
	}
	sg, err := LoadPEM("Example.COM", "", pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if sg.Domain != "example.com" || sg.Selector != "dkim" {
		t.Fatalf("unexpected signer: %+v", sg)
	}
	if !strings.HasPrefix(sg.TXT(), "v=DKIM1; k=rsa; p=") {
		t.Fatalf("bad TXT: %s", sg.TXT())
	}
	if !sg.Match("user@example.com") {
		t.Fatal("should match own domain")
	}
	if sg.Match("user@other.com") {
		t.Fatal("should not match other domain")
	}
	if sg.Match("no-at-sign") {
		t.Fatal("should not match malformed address")
	}
	if (*Signer)(nil).Match("a@b.c") {
		t.Fatal("nil signer should not match")
	}

	sig := sg.Sign([][2]string{{"From", "user@example.com"}, {"To", "x@y.z"}, {"Subject", "hi"}}, "hello")
	if !strings.HasPrefix(sig, "DKIM-Signature: ") {
		t.Fatalf("bad signature: %q", sig)
	}
	if (*Signer)(nil).Sign(nil, "x") != "" || (&Signer{}).Sign(nil, "x") != "" {
		t.Fatal("nil key should return empty signature")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dkim.pem")
	pemBytes, _ := GeneratePEM()
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("example.com", "s1", path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("", "s1", path); err == nil {
		t.Fatal("empty domain should error")
	}
	if _, err := Load("example.com", "s1", filepath.Join(dir, "missing.pem")); err == nil {
		t.Fatal("missing file should error")
	}
}

func TestLoadPEMErrors(t *testing.T) {
	if _, err := LoadPEM("example.com", "dkim", []byte("not pem")); err == nil {
		t.Fatal("bad pem should error")
	}
	if _, err := LoadPEM("", "dkim", []byte("x")); err == nil {
		t.Fatal("empty domain should error")
	}
}

func TestLoadPKCS8(t *testing.T) {
	// PKCS#8 RSA 私钥
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	sg, err := LoadPEM("example.com", "dkim", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	if sg.Sign([][2]string{{"From", "a@example.com"}}, "") == "" {
		t.Fatal("empty body should still sign")
	}

	// 非 RSA 的 PKCS#8 密钥应报错
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecDER, _ := x509.MarshalPKCS8PrivateKey(ec)
	if _, err := LoadPEM("example.com", "dkim", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})); err == nil {
		t.Fatal("non-rsa key should error")
	}
}
