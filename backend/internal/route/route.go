// Package route 解析外发邮件路由：按收件人域名匹配管理员配置的路由。
package route

import (
	"strings"

	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/secret"
)

// Match 返回最匹配且已启用的路由；无匹配返回 nil。
// 匹配：精确域名优先于后缀（.example.com）；同级按 Priority 降序、域名更长者优先。
func Match(routes []model.MailRoute, domain string) *model.MailRoute {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil
	}
	var best *model.MailRoute
	for i := range routes {
		r := &routes[i]
		if !r.Enabled {
			continue
		}
		d := strings.ToLower(strings.TrimSpace(r.Domain))
		if d == "" {
			continue
		}
		match := false
		if strings.HasPrefix(d, ".") {
			match = strings.HasSuffix(domain, d)
		} else {
			match = domain == d
		}
		if match && (best == nil || better(r, best)) {
			best = r
		}
	}
	return best
}

func better(a, b *model.MailRoute) bool {
	as := strings.HasPrefix(a.Domain, ".")
	bs := strings.HasPrefix(b.Domain, ".")
	if as != bs {
		return !as // 精确域名优先
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return len(a.Domain) > len(b.Domain)
}

// Relay 把路由转换为外发中继配置（解密密码）。
func Relay(r *model.MailRoute) runtimecfg.Relay {
	pass := ""
	if r.RelayPass != "" {
		if b, err := secret.Decrypt(r.RelayPass); err == nil {
			pass = string(b)
		}
	}
	return runtimecfg.Relay{
		Host: r.RelayHost, Port: r.RelayPort, User: r.RelayUser,
		Pass: pass, From: r.RelayFrom, Insecure: r.Insecure,
	}
}
