package handler

// DNSSEC 信息：给管理员展示需要在注册商处设置的 DS 记录与 DNSKEY。

import (
	"net/http"

	"mailserver/internal/dnsserver"
	"mailserver/internal/model"
)

type dnssecDomain struct {
	Domain string   `json:"domain"`
	DS     string   `json:"ds"`
	DNSKEY []string `json:"dnskey"`
}

// DNSSECInfo GET /api/admin/dnssec -> 各托管域名的 DS / DNSKEY（需 DNSSEC_ENABLE=1）。
func (d *DNS) DNSSECInfo(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.mustAdmin(w, r); !ok {
		return
	}
	if !d.DNSSECOn || !dnsserver.Enabled() {
		writeJSON(w, 200, map[string]any{"enabled": false, "domains": []any{}})
		return
	}
	var domains []model.Domain
	d.DB.Order("name").Find(&domains)
	out := make([]dnssecDomain, 0, len(domains))
	for _, dm := range domains {
		out = append(out, dnssecDomain{
			Domain: dm.Name,
			DS:     dnsserver.DSForZone(dm.Name),
			DNSKEY: dnsserver.DNSKEYForZone(dm.Name),
		})
	}
	writeJSON(w, 200, map[string]any{"enabled": true, "domains": out})
}
