// Package runtimecfg 保存管理后台可修改的运行时配置。
// 配置优先从数据库读取（后台写入），未设置时回退到环境变量（首次启动的引导值）。
// 这样「配置都在 admin 页面里改」，环境变量只需保留数据库/JWT 等必要引导项。
package runtimecfg

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mailserver/internal/config"
	"mailserver/internal/dkim"
	"mailserver/internal/model"
	"mailserver/internal/secret"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 配置键
const (
	KeyMailHost     = "mail_host"    // 邮件主机名（greeting / Message-ID / MX 目标）
	KeyPublicIP     = "public_ip"    // 服务器公网 IPv4
	KeyAdminEmails  = "admin_emails" // 管理员邮箱，逗号分隔
	KeyRelayHost    = "relay_host"
	KeyRelayPort    = "relay_port"
	KeyRelayUser    = "relay_user"
	KeyRelayPass    = "relay_pass"
	KeyRelayFrom    = "relay_from"
	KeyDKIMDomain   = "dkim_domain"
	KeyDKIMSel      = "dkim_selector"
	KeyDKIMKeyEnc   = "dkim_key_enc"         // AES-GCM 加密的 DKIM 私钥 PEM
	KeyRegistration = "registration_enabled" // 是否开放注册（默认关闭；首个用户始终可注册）
)

// Relay 外发中继配置。
type Relay struct {
	Host string
	Port string
	User string
	Pass string
	From string
	Name string
}

type Store struct {
	db     *gorm.DB
	mu     sync.RWMutex
	vals   map[string]string
	signer atomic.Pointer[dkim.Signer]
}

// New 从数据库加载配置，缺省项用 cfg（环境变量）补齐，并恢复 DKIM 签名器。
func New(db *gorm.DB, cfg config.Config) *Store {
	var rows []model.Setting
	if db != nil {
		db.Find(&rows)
	}
	s := newStore(cfg, rows)
	s.db = db
	return s
}

// newStore 纯逻辑：合并数据库行与环境变量默认值（便于单测）。
func newStore(cfg config.Config, rows []model.Setting) *Store {
	s := &Store{vals: map[string]string{}}
	for _, r := range rows {
		s.vals[r.Key] = r.Value
	}
	defaults := map[string]string{
		KeyMailHost:    cfg.Host,
		KeyAdminEmails: cfg.AdminEmails,
		KeyRelayHost:   cfg.RelayHost,
		KeyRelayPort:   cfg.RelayPort,
		KeyRelayUser:   cfg.RelayUser,
		KeyRelayPass:   cfg.RelayPass,
		KeyRelayFrom:   cfg.RelayFrom,
		KeyDKIMDomain:  cfg.DKIMDomain,
		KeyDKIMSel:     cfg.DKIMSelector,
	}
	for k, v := range defaults {
		if _, ok := s.vals[k]; !ok {
			s.vals[k] = v
		}
	}
	// 环境变量引导：首次启动时把 DKIM_KEY 文件内容也接管进配置
	if s.vals[KeyDKIMKeyEnc] == "" && cfg.DKIMKey != "" {
		if b, err := os.ReadFile(cfg.DKIMKey); err == nil {
			domain := s.vals[KeyDKIMDomain]
			if domain == "" {
				domain = cfg.Host
			}
			sel := s.vals[KeyDKIMSel]
			if _, err := dkim.LoadPEM(domain, sel, b); err == nil {
				if enc, err := secret.Encrypt(b); err == nil {
					s.vals[KeyDKIMKeyEnc] = enc
				}
			}
		}
	}
	_ = s.ReloadDKIM()
	return s
}

func (s *Store) get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vals[key]
}

// Get 读取配置值。
func (s *Store) Get(key string) string { return s.get(key) }

// Set 更新并持久化单个配置。
func (s *Store) Set(key, val string) error {
	s.mu.Lock()
	s.vals[key] = val
	s.mu.Unlock()
	s.persist(key, val)
	return nil
}

func (s *Store) persist(key, val string) {
	if s.db == nil { // 单元测试/无库时仅内存生效
		return
	}
	_ = s.db.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&model.Setting{Key: key, Value: val, UpdatedAt: time.Now()}).Error
}

// SetMany 批量更新。
func (s *Store) SetMany(m map[string]string) {
	for k, v := range m {
		_ = s.Set(k, v)
	}
}

func (s *Store) MailHost() string    { return s.get(KeyMailHost) }
func (s *Store) PublicIP() string    { return s.get(KeyPublicIP) }
func (s *Store) AdminEmails() string { return s.get(KeyAdminEmails) }

// RegistrationEnabled 是否开放注册（默认关闭）。
func (s *Store) RegistrationEnabled() bool { return parseBool(s.get(KeyRegistration)) }

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// Relay 返回当前外发中继配置。
func (s *Store) Relay() Relay {
	return Relay{
		Host: s.get(KeyRelayHost),
		Port: s.get(KeyRelayPort),
		User: s.get(KeyRelayUser),
		Pass: s.get(KeyRelayPass),
		From: s.get(KeyRelayFrom),
		Name: s.MailHost(),
	}
}

// Signer 返回当前 DKIM 签名器（可能为 nil）。
func (s *Store) Signer() *dkim.Signer { return s.signer.Load() }

// DKIMReady 表示是否已配置 DKIM。
func (s *Store) DKIMReady() bool { return s.signer.Load() != nil }

// SetDKIM 保存 DKIM 私钥并立即生效（加密落库）。
func (s *Store) SetDKIM(domain, selector string, keyPEM []byte) error {
	domain = normalizeDomain(domain)
	if selector == "" {
		selector = "dkim"
	}
	// 先校验可解析
	if _, err := dkim.LoadPEM(domain, selector, keyPEM); err != nil {
		return err
	}
	enc, err := secret.Encrypt(keyPEM)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.vals[KeyDKIMDomain] = domain
	s.vals[KeyDKIMSel] = selector
	s.vals[KeyDKIMKeyEnc] = enc
	s.mu.Unlock()
	s.persist(KeyDKIMDomain, domain)
	s.persist(KeyDKIMSel, selector)
	s.persist(KeyDKIMKeyEnc, enc)
	return s.ReloadDKIM()
}

// ReloadDKIM 依据当前配置重建签名器。
func (s *Store) ReloadDKIM() error {
	domain := s.get(KeyDKIMDomain)
	selector := s.get(KeyDKIMSel)
	enc := s.get(KeyDKIMKeyEnc)
	if domain == "" || enc == "" {
		s.signer.Store(nil)
		return nil
	}
	raw, err := secret.Decrypt(enc)
	if err != nil {
		s.signer.Store(nil)
		return err
	}
	sg, err := dkim.LoadPEM(domain, selector, raw)
	if err != nil {
		s.signer.Store(nil)
		return err
	}
	s.signer.Store(sg)
	return nil
}

// Snapshot 返回可在后端展示的配置（不含密钥明文）。
func (s *Store) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"mail_host":            s.vals[KeyMailHost],
		"public_ip":            s.vals[KeyPublicIP],
		"admin_emails":         s.vals[KeyAdminEmails],
		"relay_host":           s.vals[KeyRelayHost],
		"relay_port":           s.vals[KeyRelayPort],
		"relay_user":           s.vals[KeyRelayUser],
		"relay_from":           s.vals[KeyRelayFrom],
		"relay_pass_set":       s.vals[KeyRelayPass] != "",
		"dkim_domain":          s.vals[KeyDKIMDomain],
		"dkim_selector":        s.vals[KeyDKIMSel],
		"dkim_ready":           s.signer.Load() != nil,
		"registration_enabled": parseBool(s.vals[KeyRegistration]),
	}
}

func normalizeDomain(s string) string {
	out := []byte{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	// trim spaces and dots
	start, end := 0, len(out)
	for start < end && (out[start] == ' ' || out[start] == '.') {
		start++
	}
	for end > start && (out[end-1] == ' ' || out[end-1] == '.') {
		end--
	}
	return string(out[start:end])
}
