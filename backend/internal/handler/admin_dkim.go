package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mailserver/internal/dkim"
)

func (a *Admin) DKIMGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Domain   string `json:"domain"`
		Selector string `json:"selector"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	domain := normalizeDomain(in.Domain)
	if domain == "" {
		domain = deriveBaseDomain(a.RT.MailHost())
	}
	if domain == "" {
		writeJSON(w, 400, map[string]string{"error": "请先填写邮件域名"})
		return
	}
	sel := strings.TrimSpace(in.Selector)
	if sel == "" {
		sel = "dkim"
	}
	pemBytes, err := dkim.GeneratePEM()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "生成密钥失败：" + err.Error()})
		return
	}
	if err := a.RT.SetDKIM(domain, sel, pemBytes); err != nil {
		writeJSON(w, 400, map[string]string{"error": "保存 DKIM 失败：" + err.Error()})
		return
	}
	name, txt, perr := a.publishDKIM(domain)
	resp := map[string]any{"ok": "true", "domain": domain, "selector": sel, "name": name, "txt": txt}
	if perr != nil {
		resp["warning"] = "密钥已生成，但本地 DNS 记录未同步：" + perr.Error()
	}
	writeJSON(w, 200, resp)
}

// POST /api/admin/dkim/upload {domain,selector,private_key}
func (a *Admin) DKIMUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Domain     string `json:"domain"`
		Selector   string `json:"selector"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	domain := normalizeDomain(in.Domain)
	if domain == "" {
		domain = deriveBaseDomain(a.RT.MailHost())
	}
	if strings.TrimSpace(in.PrivateKey) == "" {
		writeJSON(w, 400, map[string]string{"error": "请粘贴 DKIM 私钥（PEM）"})
		return
	}
	sel := strings.TrimSpace(in.Selector)
	if sel == "" {
		sel = "dkim"
	}
	if err := a.RT.SetDKIM(domain, sel, []byte(in.PrivateKey)); err != nil {
		writeJSON(w, 400, map[string]string{"error": "私钥解析失败：" + err.Error()})
		return
	}
	name, txt, perr := a.publishDKIM(domain)
	resp := map[string]any{"ok": "true", "domain": domain, "selector": sel, "name": name, "txt": txt}
	if perr != nil {
		resp["warning"] = "密钥已保存，但本地 DNS 记录未同步：" + perr.Error()
	}
	writeJSON(w, 200, resp)
}

// ---- ACME 内部实现 ----

func (a *Admin) publishDKIM(domain string) (string, string, error) {
	if a.DNS == nil {
		return "", "", errors.New("自托管 DNS 未初始化（不影响第三方 DNS 下发）")
	}
	return a.DNS.publishDKIM(domain)
}
