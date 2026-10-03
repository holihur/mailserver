package handler

// 管理后台：第三方域名服务商（阿里云 / Cloudflare）一键下发邮件解析。
// 流程：录入凭证（加密落库）→ 校验 → 列出账号域名 → 选择域名 → 自动配齐 A/MX/SPF/DKIM/DMARC。
// 路由见 main.go：/api/admin/providers 与 /api/admin/providers/{id}/{test|domains|apply}

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/provider"
	"mailserver/internal/secret"
)

// providerCreateIn 兼容两种服务商的凭证字段。
type providerCreateIn struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	APIToken        string `json:"api_token"`
	Email           string `json:"email"`
	APIKey          string `json:"api_key"`
}

func (in providerCreateIn) creds() map[string]string {
	switch in.Type {
	case "aliyun":
		return map[string]string{
			"access_key_id":     strings.TrimSpace(in.AccessKeyID),
			"access_key_secret": strings.TrimSpace(in.AccessKeySecret),
		}
	case "cloudflare":
		return map[string]string{
			"api_token": strings.TrimSpace(in.APIToken),
			"email":     strings.TrimSpace(in.Email),
			"api_key":   strings.TrimSpace(in.APIKey),
		}
	}
	return nil
}

func (a *Admin) providerFor(p model.DnsProvider) (provider.Provider, error) {
	raw, err := secret.Decrypt(p.Creds)
	if err != nil {
		return nil, err
	}
	creds, err := provider.DecodeCreds(raw)
	if err != nil {
		return nil, err
	}
	return provider.New(p.Type, creds)
}

// GET /api/admin/providers 列表（不含凭证）
// POST /api/admin/providers {name,type,凭证...} 校验后加密保存
func (a *Admin) Providers(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		var ps []model.DnsProvider
		a.DB.Order("id ASC").Find(&ps)
		if ps == nil {
			ps = []model.DnsProvider{}
		}
		writeJSON(w, 200, ps)
	case "POST":
		var in providerCreateIn
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		in.Type = strings.ToLower(strings.TrimSpace(in.Type))
		if in.Name = strings.TrimSpace(in.Name); in.Name == "" {
			in.Name = in.Type
		}
		creds := in.creds()
		if len(creds) == 0 {
			writeJSON(w, 400, map[string]string{"error": "不支持的服务商类型（可选 aliyun / cloudflare）"})
			return
		}
		prov, err := provider.New(in.Type, creds)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := prov.Verify(ctx); err != nil {
			writeJSON(w, 400, map[string]string{"error": "凭证校验失败：" + err.Error()})
			return
		}
		raw, _ := json.Marshal(creds)
		enc, err := secret.Encrypt(raw)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "加密失败：" + err.Error()})
			return
		}
		p := model.DnsProvider{Name: in.Name, Type: in.Type, Creds: enc}
		if err := a.DB.Create(&p).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, p)
	default:
		w.WriteHeader(405)
	}
}

// /api/admin/providers/{id}
// /api/admin/providers/{id}/test     POST 校验凭证
// /api/admin/providers/{id}/domains  GET  列出账号域名
// /api/admin/providers/{id}/apply    POST {domain,ip,mail_host,publish_local} 一键配齐
func (a *Admin) ProviderOne(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/providers/"), "/")
	parts := strings.Split(rest, "/")
	id64, _ := strconv.ParseUint(parts[0], 10, 32)
	var p model.DnsProvider
	if err := a.DB.First(&p, uint(id64)).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "服务商不存在"})
		return
	}
	if len(parts) == 1 {
		if r.Method == "DELETE" {
			a.DB.Delete(&p)
			writeJSON(w, 200, map[string]string{"ok": "true"})
			return
		}
		w.WriteHeader(405)
		return
	}

	prov, err := a.providerFor(p)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "凭证解密失败（JWT_SECRET 是否变更过？）：" + err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch parts[1] {
	case "test":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if err := prov.Verify(ctx); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "true"})
	case "domains":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		zs, err := prov.ListZones(ctx)
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		if zs == nil {
			zs = []provider.Zone{}
		}
		writeJSON(w, 200, zs)
	case "apply":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		a.applyProvider(w, r, prov)
	default:
		w.WriteHeader(404)
	}
}

func (a *Admin) applyProvider(w http.ResponseWriter, r *http.Request, prov provider.Provider) {
	var in struct {
		Domain       string            `json:"domain"`
		IP           string            `json:"ip"`
		MailHost     string            `json:"mail_host"`
		Records      []provider.Record `json:"records"`
		PublishLocal *bool             `json:"publish_local"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	domain := strings.ToLower(strings.Trim(strings.TrimSpace(in.Domain), "."))
	if domain == "" {
		writeJSON(w, 400, map[string]string{"error": "请选择要配置的域名"})
		return
	}
	ip := strings.TrimSpace(in.IP)
	if ip == "" {
		v, err := detectPublicIP(r.Context())
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "请填写服务器公网 IP（自动探测失败：" + err.Error() + "）"})
			return
		}
		ip = v
	}
	if parsed := net.ParseIP(ip); parsed == nil || parsed.To4() == nil {
		writeJSON(w, 400, map[string]string{"error": "公网 IPv4 格式不正确（暂不支持 IPv6）：" + ip})
		return
	}

	// DKIM：签名域匹配时用真实公钥，否则占位
	selector, dkimTXT := "dkim", ""
	if a.DNS != nil && a.DNS.Signer != nil && strings.EqualFold(a.DNS.Signer.Domain, domain) {
		selector = a.DNS.Signer.Selector
		dkimTXT = a.DNS.Signer.TXT()
	}

	recs := in.Records
	var localRecs []model.DnsRecord
	if len(recs) == 0 {
		localRecs = BuildMailRecords(domain, ip, in.MailHost, selector, dkimTXT)
		for _, lr := range localRecs {
			recs = append(recs, provider.Record{Name: lr.Name, Type: lr.Type, Value: lr.Value, TTL: lr.TTL, Priority: lr.Prio})
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	results, err := prov.EnsureRecords(ctx, domain, recs)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	// 本地同步一份（供后台继续建邮箱账号 / DnsPage 展示），默认开启
	publish := true
	if in.PublishLocal != nil {
		publish = *in.PublishLocal
	}
	resp := map[string]any{"results": results, "domain": domain, "ip": ip}
	if publish && a.DNS != nil {
		if len(localRecs) == 0 {
			for _, rr := range recs {
				localRecs = append(localRecs, model.DnsRecord{Name: rr.Name, Type: rr.Type, Value: rr.Value, TTL: rr.TTL, Prio: rr.Priority})
			}
		}
		if _, err := a.DNS.UpsertDomainWithRecords(domain, localRecs); err != nil {
			resp["warning"] = "记录已下发，但本地同步失败：" + err.Error()
		}
	}
	writeJSON(w, 200, resp)
}

// detectPublicIP 依次尝试公共 IP 查询服务，失败则返回错误由前端要求手填。
func detectPublicIP(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, u := range []string{"https://api.ipify.org", "https://ifconfig.me/ip", "https://ipinfo.io/ip"} {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		ip := strings.TrimSpace(string(b))
		if net.ParseIP(ip) != nil {
			return ip, nil
		}
	}
	return "", errors.New("网络不可达")
}
