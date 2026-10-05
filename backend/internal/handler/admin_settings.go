package handler

// 管理后台：系统设置 + TLS/SSL（手动上传 / Let's Encrypt 自动签发）+ DKIM 密钥管理。
// 目标是「所有配置都在后台点」，命令行只保留数据库/JWT 等必要引导项。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/certstore"
	"mailserver/internal/dkim"
	"mailserver/internal/letsencrypt"
	"mailserver/internal/model"
	"mailserver/internal/provider"
	"mailserver/internal/runtimecfg"
)

var settingKeys = map[string]bool{
	runtimecfg.KeyMailHost:         true,
	runtimecfg.KeyPublicIP:         true,
	runtimecfg.KeyAdminEmails:      true,
	runtimecfg.KeyRelayHost:        true,
	runtimecfg.KeyRelayPort:        true,
	runtimecfg.KeyRelayUser:        true,
	runtimecfg.KeyRelayPass:        true,
	runtimecfg.KeyRelayFrom:        true,
	runtimecfg.KeyRelayInsecure:    true,
	runtimecfg.KeyDirectSend:       true,
	runtimecfg.KeyRegistration:     true,
	runtimecfg.KeyAutoUpdate:       true,
	runtimecfg.KeyUpdateInterval:   true,
	runtimecfg.KeyOIDCEnabled:      true,
	runtimecfg.KeyOIDCIssuer:       true,
	runtimecfg.KeyOIDCClientID:     true,
	runtimecfg.KeyOIDCClientSecret: true,
	runtimecfg.KeyOIDCAutoCreate:   true,
	runtimecfg.KeyBackupDir:        true,
	runtimecfg.KeyBackupInterval:   true,
	runtimecfg.KeyBackupKeep:       true,
}

// GET /api/admin/settings   PATCH /api/admin/settings
func (a *Admin) Settings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, a.RT.Snapshot())
	case "PATCH":
		var in map[string]string
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd := map[string]string{}
		for k, v := range in {
			if !settingKeys[k] {
				continue
			}
			v = strings.TrimSpace(v)
			// 密码留空表示不修改
			if k == runtimecfg.KeyRelayPass && v == "" {
				continue
			}
			if k == runtimecfg.KeyOIDCClientSecret && v == "" {
				continue
			}
			if k == runtimecfg.KeyMailHost {
				v = strings.Trim(strings.ToLower(v), ".")
			}
			if k == runtimecfg.KeyRegistration || k == runtimecfg.KeyRelayInsecure || k == runtimecfg.KeyDirectSend || k == runtimecfg.KeyAutoUpdate || k == runtimecfg.KeyOIDCEnabled || k == runtimecfg.KeyOIDCAutoCreate {
				if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on") {
					v = "1"
				} else {
					v = "0"
				}
			}
			if k == runtimecfg.KeyUpdateInterval {
				n, err := strconv.Atoi(v)
				if err != nil || n <= 0 {
					n = 10
				}
				if n > 1440 {
					n = 1440
				}
				v = strconv.Itoa(n)
			}
			if k == runtimecfg.KeyBackupInterval {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 {
					n = 24
				}
				if n > 8760 {
					n = 8760
				}
				v = strconv.Itoa(n)
			}
			if k == runtimecfg.KeyBackupKeep {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 {
					n = 7
				}
				if n > 3650 {
					n = 3650
				}
				v = strconv.Itoa(n)
			}
			upd[k] = v
		}
		if h, ok := upd[runtimecfg.KeyMailHost]; ok && h != "" && !strings.Contains(h, ".") {
			writeJSON(w, 400, map[string]string{"error": "邮件域名格式不正确，例如 mail.example.com"})
			return
		}
		a.RT.SetMany(upd)
		writeJSON(w, 200, a.RT.Snapshot())
	default:
		w.WriteHeader(405)
	}
}

// GET /api/admin/tls  -> 证书状态 + ACME 配置 + 可选服务商
// DELETE /api/admin/tls -> 删除证书
func (a *Admin) TLS(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if a.Cert == nil {
		writeJSON(w, 500, map[string]string{"error": "证书存储未初始化"})
		return
	}
	switch r.Method {
	case "GET":
		resp := map[string]any{"cert": a.Cert.Meta()}
		var ac model.AcmeConfig
		if err := a.DB.Order("id").First(&ac).Error; err == nil {
			resp["acme"] = ac
		}
		var ps []model.DnsProvider
		a.DB.Order("id").Find(&ps)
		if ps == nil {
			ps = []model.DnsProvider{}
		}
		resp["providers"] = ps
		writeJSON(w, 200, resp)
	case "DELETE":
		if err := a.Cert.Clear(); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(405)
	}
}

// POST /api/admin/tls/manual {cert,key}
func (a *Admin) TLSManual(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if a.Cert == nil {
		writeJSON(w, 500, map[string]string{"error": "证书存储未初始化"})
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Cert string `json:"cert"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	if strings.TrimSpace(in.Cert) == "" || strings.TrimSpace(in.Key) == "" {
		writeJSON(w, 400, map[string]string{"error": "证书与私钥都不能为空"})
		return
	}
	if err := a.Cert.Set([]byte(in.Cert), []byte(in.Key), "manual"); err != nil {
		writeJSON(w, 400, map[string]string{"error": "证书解析失败：" + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": "true", "cert": a.Cert.Meta()})
}

// POST /api/admin/tls/acme {domain,email,provider_id,auto_renew,staging}
func (a *Admin) TLSAcme(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Domain     string `json:"domain"`
		Email      string `json:"email"`
		ProviderID uint   `json:"provider_id"`
		AutoRenew  bool   `json:"auto_renew"`
		Staging    bool   `json:"staging"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	in.Domain = strings.Trim(strings.ToLower(strings.TrimSpace(in.Domain)), ".")
	if in.Domain == "" || !strings.Contains(in.Domain, ".") {
		writeJSON(w, 400, map[string]string{"error": "请填写要签发证书的域名，例如 mail.example.com"})
		return
	}
	if in.ProviderID == 0 {
		writeJSON(w, 400, map[string]string{"error": "请选择用于 DNS 质询的域名服务商"})
		return
	}
	cfg := model.AcmeConfig{Domain: in.Domain, ProviderID: in.ProviderID, Email: in.Email, AutoRenew: in.AutoRenew, UpdatedAt: time.Now()}
	a.DB.Where("1 = 1").Delete(&model.AcmeConfig{})
	if err := a.DB.Create(&cfg).Error; err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存 ACME 配置失败"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	meta, err := a.issueACME(ctx, cfg, in.Staging)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": "true", "cert": meta, "acme": cfg})
}

// POST /api/admin/dkim/generate {domain,selector}
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

func (a *Admin) issueACME(ctx context.Context, cfg model.AcmeConfig, staging bool) (certstore.Meta, error) {
	solver, err := a.buildSolver(ctx, cfg.ProviderID, cfg.Domain)
	if err != nil {
		return certstore.Meta{}, err
	}
	issuer, err := letsencrypt.NewIssuer(a.CertDir, staging)
	if err != nil {
		return certstore.Meta{}, err
	}
	certPEM, keyPEM, err := issuer.Issue(ctx, []string{cfg.Domain}, cfg.Email, solver)
	if err != nil {
		return certstore.Meta{}, err
	}
	if err := a.Cert.Set(certPEM, keyPEM, "acme"); err != nil {
		return certstore.Meta{}, err
	}
	return a.Cert.Meta(), nil
}

func (a *Admin) buildSolver(ctx context.Context, providerID uint, domain string) (letsencrypt.Solver, error) {
	var p model.DnsProvider
	if err := a.DB.First(&p, providerID).Error; err != nil {
		return nil, errors.New("所选域名服务商不存在")
	}
	prov, err := a.providerFor(p)
	if err != nil {
		return nil, err
	}
	zones, err := prov.ListZones(ctx)
	if err != nil {
		return nil, err
	}
	z, ok := provider.MatchZone(zones, domain)
	if !ok {
		return nil, fmt.Errorf("服务商账号下没有包含 %s 的域名，请先在服务商处添加再重试", domain)
	}
	return &providerSolver{prov: prov, zone: z.Name}, nil
}

type providerSolver struct {
	prov provider.Provider
	zone string
}

func (s *providerSolver) Present(ctx context.Context, recordName, value string) error {
	rel := provider.RelativeName(s.zone, recordName)
	_, err := s.prov.EnsureRecords(ctx, s.zone, []provider.Record{{Name: rel, Type: "TXT", Value: value, TTL: 60}})
	return err
}

func (s *providerSolver) CleanUp(ctx context.Context, recordName, value string) error {
	rel := provider.RelativeName(s.zone, recordName)
	return s.prov.DeleteRecord(ctx, s.zone, rel, "TXT", value)
}

// Wait 轮询公共 DNS，尽量等质询记录生效；超时也返回 nil，交由 ACME 服务器判定。
func (s *providerSolver) Wait(ctx context.Context, recordName, value string) error {
	r := net.Resolver{}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
		txts, err := r.LookupTXT(ctx, recordName)
		if err == nil {
			for _, t := range txts {
				if strings.TrimSpace(t) == value {
					return nil
				}
			}
		}
	}
	return nil
}

// AutoRenewLoop 定期检查并按需续期 ACME 证书。
func (a *Admin) AutoRenewLoop(ctx context.Context) {
	a.tryRenew(ctx)
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.tryRenew(ctx)
		}
	}
}

func (a *Admin) tryRenew(ctx context.Context) {
	if a.Cert == nil {
		return
	}
	var cfg model.AcmeConfig
	if err := a.DB.Order("id").First(&cfg).Error; err != nil || !cfg.AutoRenew {
		return
	}
	meta := a.Cert.Meta()
	if meta.Source == "manual" {
		return // 手动证书不覆盖
	}
	if meta.Exists && time.Until(meta.NotAfter) > 30*24*time.Hour {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := a.issueACME(cctx, cfg, false); err != nil {
		log.Println("acme renew:", err)
		return
	}
	log.Println("acme: 证书已自动续期", cfg.Domain)
}

func normalizeDomain(s string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".")
}

// deriveBaseDomain mail.example.com -> example.com；example.com -> example.com
func deriveBaseDomain(host string) string {
	h := strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "" {
		return ""
	}
	parts := strings.Split(h, ".")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return h
}
