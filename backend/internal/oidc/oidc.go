// Package oidc 实现 OpenID Connect 授权码流程所需的最小客户端（发现 + 换 token + userinfo）。
// 仅用标准库，避免额外依赖。
package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Provider struct {
	Issuer      string
	AuthURL     string
	TokenURL    string
	UserInfoURL string
}

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
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
	return &Provider{Issuer: d.Issuer, AuthURL: d.AuthorizationEndpoint, TokenURL: d.TokenEndpoint, UserInfoURL: d.UserinfoEndpoint}, nil
}

// AuthCodeURL 构造授权跳转地址。
func (p *Provider) AuthCodeURL(clientID, redirectURI, state string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("scope", "openid email profile")
	v.Set("state", state)
	sep := "?"
	if strings.Contains(p.AuthURL, "?") {
		sep = "&"
	}
	return p.AuthURL + sep + v.Encode()
}

// Exchange 用授权码换 access_token。
func (p *Provider) Exchange(ctx context.Context, clientID, clientSecret, code, redirectURI string) (string, error) {
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
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("未返回 access_token")
	}
	return tr.AccessToken, nil
}

type UserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// UserInfo 用 access_token 拉取用户信息。
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
	if strings.TrimSpace(ui.Email) == "" {
		return nil, fmt.Errorf("userinfo 未返回 email")
	}
	return &ui, nil
}
