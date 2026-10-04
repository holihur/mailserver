package handler

// 投递可达性建议记录：MTA-STS / TLS-RPT / DANE(TLSA)。

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mailserver/internal/model"
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

// PublishDeliverability POST /api/admin/deliverability {domain_id} -> 一键写入建议记录。
func (d *DNS) PublishDeliverability(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		DomainID uint `json:"domain_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	var dm model.Domain
	if err := d.DB.First(&dm, in.DomainID).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "domain not found"})
		return
	}
	host := "mail." + dm.Name
	if d.RT != nil {
		mh := strings.ToLower(d.RT.MailHost())
		if mh != "" && strings.HasSuffix(mh, "."+strings.ToLower(dm.Name)) {
			host = mh
		}
	}
	recs := []model.DnsRecord{
		{Name: "_mta-sts", Type: "TXT", Value: "v=STSv1; id=" + time.Now().Format("20060102150405"), TTL: 600},
		{Name: "_smtp._tls", Type: "TXT", Value: "v=TLSRPTv1; rua=mailto:postmaster@" + dm.Name, TTL: 600},
	}
	if tlsa := d.tlsa(host); tlsa != "" {
		rel := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(host), "."), "."+strings.ToLower(dm.Name))
		recs = append(recs, model.DnsRecord{Name: "_25._tcp." + rel, Type: "TLSA", Value: tlsa, TTL: 600})
	}
	created := 0
	for _, rec := range recs {
		rec.DomainID = dm.ID
		var n int64
		d.DB.Model(&model.DnsRecord{}).Where("domain_id = ? AND name = ? AND type = ?", dm.ID, rec.Name, rec.Type).Count(&n)
		if n == 0 {
			d.DB.Create(&rec)
			created++
		}
	}
	d.export()
	writeJSON(w, 200, map[string]any{"ok": true, "created": created})
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
