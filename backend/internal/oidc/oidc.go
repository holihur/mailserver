// Package oidc 实现 OpenID Connect 授权码流程（严格模式）：
// 发现 → 授权码 → 换 token → **校验 id_token 签名（JWKS）+ iss/aud/exp/nonce** → 取用户信息。
// 仅用标准库 + golang-jwt 校验 JWT。
package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Provider struct {
	Issuer      string
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	JWKSURL     string
}

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// Discover 读取 issuer 的 .well-known/openid-configuration。
func Discover(ctx context.Context, issuer string) (*Provider, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return nil, fmt.Errorf("issuer 为空")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery HTTP %d", resp.StatusCode)
	}
	var d discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return nil, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return nil, fmt.Errorf("discovery 缺少端点")
	}
	iss := d.Issuer
	if iss == "" {
		iss = issuer
	}
	return &Provider{Issuer: iss, AuthURL: d.AuthorizationEndpoint, TokenURL: d.TokenEndpoint, UserInfoURL: d.UserinfoEndpoint, JWKSURL: d.JwksURI}, nil
}

// AuthCodeURL 构造授权跳转地址（含 nonce）。
func (p *Provider) AuthCodeURL(clientID, redirectURI, state, nonce string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("scope", "openid email profile")
	v.Set("state", state)
	v.Set("nonce", nonce)
	sep := "?"
	if strings.Contains(p.AuthURL, "?") {
		sep = "&"
	}
	return p.AuthURL + sep + v.Encode()
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

// Exchange 用授权码换 access_token 与 id_token。
func (p *Provider) Exchange(ctx context.Context, clientID, clientSecret, code, redirectURI string) (accessToken, idToken string, err error) {
	v := url.Values{}
	v.Set("grant_type", "authorization_code")
	v.Set("code", code)
	v.Set("redirect_uri", redirectURI)
	v.Set("client_id", clientID)
	v.Set("client_secret", clientSecret)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("token HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tr tokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", "", err
	}
	if tr.IDToken == "" {
		return "", "", fmt.Errorf("未返回 id_token（严格模式要求 openid scope）")
	}
	return tr.AccessToken, tr.IDToken, nil
}

// UserInfo 用 access_token 拉取用户信息（辅助）。
func (p *Provider) UserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	if p.UserInfoURL == "" {
		return nil, fmt.Errorf("无 userinfo 端点")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo HTTP %d", resp.StatusCode)
	}
	var ui UserInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ui); err != nil {
		return nil, err
	}
	return &ui, nil
}

type UserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"email_verified"`
}

// VerifyIDToken 严格校验 id_token：签名（JWKS）、iss、aud、exp，并核对 nonce。
func (p *Provider) VerifyIDToken(ctx context.Context, idToken, clientID, nonce string) (*UserInfo, error) {
	if p.JWKSURL == "" {
		return nil, fmt.Errorf("provider 未提供 jwks_uri")
	}
	keyfunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
			}
		}
		kid, _ := t.Header["kid"].(string)
		keys, err := fetchJWKS(ctx, p.JWKSURL)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if kid == "" || k.Kid == kid {
				return k.publicKey()
			}
		}
		return nil, fmt.Errorf("未找到匹配的签名密钥")
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(idToken, claims, keyfunc,
		jwt.WithIssuer(p.Issuer),
		jwt.WithAudience(clientID),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(60*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("id_token 校验失败: %w", err)
	}
	if nonce != "" {
		if n, _ := claims["nonce"].(string); n != nonce {
			return nil, fmt.Errorf("nonce 不匹配")
		}
	}
	email, _ := claims["email"].(string)
	if strings.TrimSpace(email) == "" {
		return nil, fmt.Errorf("id_token 未包含 email")
	}
	sub, _ := claims["sub"].(string)
	name, _ := claims["name"].(string)
	verified, _ := claims["email_verified"].(bool)
	return &UserInfo{Sub: sub, Email: email, Name: name, EmailVerified: verified}, nil
}

// ---------- JWKS ----------

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

var (
	jwksMu    sync.Mutex
	jwksCache = map[string]struct {
		keys []jwk
		at   time.Time
	}{}
)

func fetchJWKS(ctx context.Context, url string) ([]jwk, error) {
	jwksMu.Lock()
	if c, ok := jwksCache[url]; ok && time.Since(c.at) < time.Hour {
		jwksMu.Unlock()
		return c.keys, nil
	}
	jwksMu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks HTTP %d", resp.StatusCode)
	}
	var out struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	jwksMu.Lock()
	jwksCache[url] = struct {
		keys []jwk
		at   time.Time
	}{out.Keys, time.Now()}
	jwksMu.Unlock()
	return out.Keys, nil
}

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "RSA":
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("不支持的曲线: %s", k.Crv)
		}
		xb, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		yb, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, nil
	}
	return nil, fmt.Errorf("不支持的密钥类型: %s", k.Kty)
}
