package handler

// OIDC 单点登录：/api/oidc/login 跳转授权，/api/oidc/callback 换 token 并登录。
// 配置（issuer / client_id / client_secret / auto_create）在管理后台「邮件主机」设置里。

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/model"
	"mailserver/internal/oidc"

	"golang.org/x/crypto/bcrypt"
)

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Auth) oidcRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/oidc/callback"
}

func (a *Auth) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	if a.RT == nil || !a.RT.OIDCEnabled() || a.RT.OIDCIssuer() == "" {
		writeJSON(w, 404, map[string]string{"error": "OIDC 未启用"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	p, err := oidc.Discover(ctx, a.RT.OIDCIssuer())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "OIDC 发现失败: " + err.Error()})
		return
	}
	state := randToken()
	nonce := randToken()
	http.SetCookie(w, &http.Cookie{
		Name: "oidc_state", Value: state, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 600, Secure: r.TLS != nil,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "oidc_nonce", Value: nonce, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 600, Secure: r.TLS != nil,
	})
	http.Redirect(w, r, p.AuthCodeURL(a.RT.OIDCClientID(), a.oidcRedirectURI(r), state, nonce), http.StatusFound)
}

func (a *Auth) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if a.RT == nil || !a.RT.OIDCEnabled() {
		writeJSON(w, 404, map[string]string{"error": "OIDC 未启用"})
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Redirect(w, r, "/login?oidc_error="+url.QueryEscape(e), http.StatusFound)
		return
	}
	if c, err := r.Cookie("oidc_state"); err != nil || c.Value == "" || c.Value != q.Get("state") {
		http.Redirect(w, r, "/login?oidc_error=state", http.StatusFound)
		return
	}
	nonce := ""
	if c, err := r.Cookie("oidc_nonce"); err == nil {
		nonce = c.Value
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	p, err := oidc.Discover(ctx, a.RT.OIDCIssuer())
	if err != nil {
		http.Redirect(w, r, "/login?oidc_error=discovery", http.StatusFound)
		return
	}
	_, idToken, err := p.Exchange(ctx, a.RT.OIDCClientID(), a.RT.OIDCClientSecret(), q.Get("code"), a.oidcRedirectURI(r))
	if err != nil {
		http.Redirect(w, r, "/login?oidc_error=token", http.StatusFound)
		return
	}
	// 严格校验 id_token：签名(JWKS) + iss/aud/exp + nonce
	ui, err := p.VerifyIDToken(ctx, idToken, a.RT.OIDCClientID(), nonce)
	if err != nil {
		http.Redirect(w, r, "/login?oidc_error=idtoken", http.StatusFound)
		return
	}
	email := strings.ToLower(strings.TrimSpace(ui.Email))
	var u model.User
	if err := a.DB.Where("LOWER(email) = ?", email).First(&u).Error; err != nil {
		if !a.RT.OIDCAutoCreate() {
			http.Redirect(w, r, "/login?oidc_error=no_account", http.StatusFound)
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(randToken()), bcrypt.DefaultCost)
		name := strings.TrimSpace(ui.Name)
		if name == "" {
			name = strings.Split(email, "@")[0]
		}
		u = model.User{Email: email, Name: name, PassHash: string(hash)}
		var n int64
		a.DB.Model(&model.User{}).Count(&n)
		if n > 0 {
			at := strings.LastIndex(email, "@")
			if at <= 0 || !a.isHostedEmailDomain(email[at+1:]) {
				http.Redirect(w, r, "/login?oidc_error=domain", http.StatusFound)
				return
			}
		}
		if n == 0 || isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), email) {
			u.Admin = true
		}
		if err := a.DB.Create(&u).Error; err != nil {
			http.Redirect(w, r, "/login?oidc_error=create", http.StatusFound)
			return
		}
	}
	if u.Disabled {
		http.Redirect(w, r, "/login?oidc_error=disabled", http.StatusFound)
		return
	}
	if !u.Admin && isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), u.Email) {
		u.Admin = true
		a.DB.Model(&u).Update("admin", true)
	}
	jwt, _ := auth.Sign(u.ID, u.Email, u.TokenVersion)
	http.Redirect(w, r, "/?oidc_token="+url.QueryEscape(jwt), http.StatusFound)
}
