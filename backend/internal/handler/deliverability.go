package handler

// 投递可达性建议记录：MTA-STS / TLS-RPT / DANE(TLSA)。

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dnsHint struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Deliverability GET /api/admin/deliverability -> 建议在 DNS 里添加的投递可达性记录。
func (d *DNS) Deliverability(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.mustAdmin(w, r); !ok {
		return
	}
	host := "mail.example.com"
	if d.RT != nil && d.RT.MailHost() != "" {
		host = d.RT.MailHost()
	}
	domain := host
	if i := strings.IndexByte(host, '.'); i > 0 && strings.Contains(host[i+1:], ".") {
		domain = host[i+1:]
	}
	out := []dnsHint{
		{Name: "_mta-sts." + domain, Type: "TXT", Value: "v=STSv1; id=" + time.Now().Format("20060102150405")},
		{Name: "_smtp._tls." + domain, Type: "TXT", Value: "v=TLSRPTv1; rua=mailto:postmaster@" + domain},
	}
	if tlsa := d.tlsa(host); tlsa != "" {
		out = append(out, dnsHint{Name: "_25._tcp." + host, Type: "TLSA", Value: tlsa})
	}
	writeJSON(w, 200, out)
}

// tlsa 从证书目录的 cert.pem 计算 SPKI sha256（3 1 1）。
func (d *DNS) tlsa(host string) string {
	if d.CertDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(d.CertDir, "cert.pem"))
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	spki, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(spki)
	return "3 1 1 " + hex.EncodeToString(sum[:])
}
