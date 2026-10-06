package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/certstore"
	"mailserver/internal/letsencrypt"
	"mailserver/internal/model"
	"mailserver/internal/provider"
)

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
