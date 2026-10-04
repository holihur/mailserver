package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

// MailTokenPrefix 应用专用密码（PAT）的固定前缀，便于识别与排查。
const MailTokenPrefix = "mst_"

// PAT 授权范围（按协议）。空 Scopes 表示不限制（兼容旧令牌）。
const (
	ScopeIMAP  = "imap"  // IMAP 同步
	ScopePOP3  = "pop3"  // POP3 取信
	ScopeSMTP  = "smtp"  // SMTP 发信
	ScopeJMAP  = "jmap"  // JMAP
	ScopeSieve = "sieve" // ManageSieve 脚本
	ScopeMCP   = "mcp"   // MCP（AI 客户端）
)

var scopeOrder = []string{ScopeIMAP, ScopePOP3, ScopeSMTP, ScopeJMAP, ScopeSieve, ScopeMCP}

var allowedScopes = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range scopeOrder {
		m[s] = true
	}
	return m
}()

// ValidScopes 校验逗号分隔的授权范围列表（保存前调用）。
func ValidScopes(s string) error {
	for _, c := range strings.Split(s, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if !allowedScopes[c] {
			return fmt.Errorf("未知的授权范围: %s", c)
		}
	}
	return nil
}

// NormalizeScopes 规整授权范围：小写、去重、按固定顺序输出；空=不限。
func NormalizeScopes(s string) string {
	set := map[string]bool{}
	for _, c := range strings.Split(s, ",") {
		if c = strings.ToLower(strings.TrimSpace(c)); c != "" {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for _, s := range scopeOrder {
		if set[s] {
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}

// TokenAllows 判断令牌是否授权所需范围（空 Scopes=不限，兼容旧令牌）。
func TokenAllows(scopes, required string) bool {
	scopes = strings.TrimSpace(scopes)
	if scopes == "" {
		return true
	}
	for _, c := range strings.Split(scopes, ",") {
		if strings.EqualFold(strings.TrimSpace(c), required) {
			return true
		}
	}
	return false
}

// NewMailToken 生成一个新 PAT。返回明文（只应展示一次）与用于界面展示的 prefix。
func NewMailToken() (plain, prefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = MailTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	prefix = plain[:12]
	return plain, prefix, nil
}

// HashMailToken 计算令牌摘要。令牌是 256bit 随机值，sha256 足以防止库泄露后被反推，
// 且可确定性索引查找（不像 bcrypt 无法按值查询）。
func HashMailToken(plain string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plain)))
	return hex.EncodeToString(sum[:])
}

// ValidCIDRs 校验逗号分隔的 CIDR/IP 列表（保存前调用）。
func ValidCIDRs(s string) error {
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if net.ParseIP(c) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("非法的 CIDR/IP: %s", c)
		}
	}
	return nil
}

// ipAllowed 判断来源 IP 是否在允许的 CIDR 列表内（空列表=不限）。
func ipAllowed(cidrs, ip string) bool {
	cidrs = strings.TrimSpace(cidrs)
	if cidrs == "" {
		return true
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, c := range strings.Split(cidrs, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			if p := net.ParseIP(c); p != nil && p.Equal(parsed) {
				return true
			}
			continue
		}
		if _, ipnet, err := net.ParseCIDR(c); err == nil && ipnet.Contains(parsed) {
			return true
		}
	}
	return false
}

// AuthenticateMail 校验邮件客户端 / MCP 的 PAT 凭据（不接受网页登录密码）。
// ip 为来源地址（可为空，为空时跳过 CIDR 限制校验）。
// required 为本次访问所需的授权范围（可多个），任一不匹配则拒绝；空则不校验范围。
func AuthenticateMail(db *gorm.DB, email, secret, ip string, required ...string) (*model.User, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, MailTokenPrefix) {
		return nil, errors.New("请使用「应用专用密码」(PAT)，不要使用网页登录密码")
	}
	var tok model.MailToken
	if err := db.Where("hash = ?", HashMailToken(secret)).First(&tok).Error; err != nil {
		return nil, errors.New("应用专用密码无效")
	}
	if strings.TrimSpace(ip) != "" && !ipAllowed(tok.AllowedCIDRs, ip) {
		return nil, errors.New("来源 IP 不在该应用专用密码允许的范围内")
	}
	for _, sc := range required {
		if sc == "" {
			continue
		}
		if !TokenAllows(tok.Scopes, sc) {
			return nil, fmt.Errorf("该应用专用密码未授权 %s 访问", sc)
		}
	}
	var u model.User
	if err := db.First(&u, tok.UserID).Error; err != nil {
		return nil, errors.New("应用专用密码无效")
	}
	if u.Disabled {
		return nil, errors.New("账号已禁用")
	}
	if e := strings.TrimSpace(email); e != "" && !strings.EqualFold(e, u.Email) {
		return nil, errors.New("应用专用密码与账号不匹配")
	}
	now := time.Now()
	db.Model(&model.MailToken{}).Where("id = ?", tok.ID).Update("last_used", now)
	return &u, nil
}

// HostOf 从 "host:port" 取出 host。
func HostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
