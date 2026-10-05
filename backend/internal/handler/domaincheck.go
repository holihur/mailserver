package handler

// 新手引导用的域名解析预检（#46）：实时查询 MX / SPF / DKIM / DMARC，
// 并判断 MX 是否指向本站配置的邮件主机。仅管理员可用。

import (
	"net"
	"net/http"
	"strings"

	"mailserver/internal/runtimecfg"
)

// GET /api/admin/domain-check?domain=example.com
func (a *Admin) DomainCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain == "" || !strings.Contains(domain, ".") {
		writeJSON(w, 400, map[string]string{"error": "请输入有效域名，例如 example.com"})
		return
	}
	expected := ""
	if a.RT != nil {
		expected = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(a.RT.MailHost())), ".")
	}
	var mx []string
	if mxs, err := net.LookupMX(domain); err == nil {
		for _, m := range mxs {
			mx = append(mx, strings.TrimSuffix(strings.ToLower(m.Host), "."))
		}
	}
	mxOK := false
	for _, m := range mx {
		if expected != "" && m == expected {
			mxOK = true
		}
	}
	spf, _ := lookupTXT(domain, "v=spf1")
	dmarc, _ := lookupTXT("_dmarc."+domain, "v=dmarc1")
	selector := "dkim"
	if a.RT != nil {
		if s := strings.TrimSpace(a.RT.Get(runtimecfg.KeyDKIMSel)); s != "" {
			selector = s
		}
	}
	dkim, _ := lookupTXT(selector+"._domainkey."+domain, "v=dkim1")

	writeJSON(w, 200, map[string]any{
		"domain":      domain,
		"expected_mx": expected,
		"mx":          mx,
		"mx_ok":       mxOK,
		"spf":         spf != "",
		"spf_value":   spf,
		"dkim":        dkim != "",
		"dkim_value":  dkim,
		"selector":    selector,
		"dmarc":       dmarc != "",
		"dmarc_value": dmarc,
	})
}

// lookupTXT 返回 domain 下第一条以 prefix（大小写不敏感）开头的 TXT 记录。
func lookupTXT(domain, prefix string) (string, error) {
	txts, err := net.LookupTXT(domain)
	if err != nil {
		return "", err
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), prefix) {
			return t, nil
		}
	}
	return "", nil
}
