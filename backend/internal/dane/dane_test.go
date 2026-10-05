package dane

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func selfSigned(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mx.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// #17：DANE-EE（usage=3）整证书 / SPKI 的 SHA-256 匹配。
func TestVerify(t *testing.T) {
	cert := selfSigned(t)
	sum := sha256.Sum256(cert.Raw)
	recs := []Record{{Usage: 3, Selector: 0, MatchingType: 1, Cert: sum[:]}}
	if err := Verify(recs, []*x509.Certificate{cert}); err != nil {
		t.Fatalf("整证书匹配应通过: %v", err)
	}

	spki := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	recs = []Record{{Usage: 3, Selector: 1, MatchingType: 1, Cert: spki[:]}}
	if err := Verify(recs, []*x509.Certificate{cert}); err != nil {
		t.Fatalf("SPKI 匹配应通过: %v", err)
	}

	recs = []Record{{Usage: 3, Selector: 1, MatchingType: 1, Cert: make([]byte, 32)}}
	if err := Verify(recs, []*x509.Certificate{cert}); err == nil {
		t.Fatalf("不匹配的 TLSA 应报错")
	}
	if err := Verify(nil, []*x509.Certificate{cert}); err == nil {
		t.Fatalf("无记录应报错")
	}
	if err := Verify(recs, nil); err == nil {
		t.Fatalf("无证书应报错")
	}
}
