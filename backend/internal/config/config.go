package config

import (
	"os"
	"strings"
)

type Config struct {
	Port          string
	SMTPport      string // 2525 本地入站（联调用）
	SubmitPort    string // 587 客户端提交（AUTH 必需）
	SubmitTLSPort string // 465 隐式 TLS，为空则不监听
	Pop3Port      string // 110 客户端取信
	Pop3TLSPort   string // 995 隐式 TLS，为空则不监听
	ImapPort      string // 143 客户端同步
	ImapTLSPort   string // 993 隐式 TLS，为空则不监听
	DKIMDomain    string // DKIM 签名域，默认 MAIL_HOST 去首段
	DKIMSelector  string
	DKIMKey       string // DKIM RSA 私钥 pem，为空则不签名
	TLSCert       string // 证书 pem，为空则 587/110 明文可用
	TLSKey        string
	Host          string // 本机邮件域名，用于 greeting/Message-ID
	AdminEmails   string // ADMIN_EMAILS 逗号分隔，命中即管理员（兜底提权）
	JWTSecret     string
	DBPath        string
	RelayHost     string // 25 被封时的外发中继（587+STARTTLS）
	RelayPort     string
	RelayUser     string
	RelayPass     string
	RelayFrom     string
}

// DKIM_DOMAIN 未配置时从 MAIL_HOST 推导：mail.example.com -> example.com
func dkimDomain(explicit, host string) string {
	if explicit != "" {
		return explicit
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if i := strings.IndexByte(host, '.'); i > 0 && strings.Contains(host[i+1:], ".") {
		return host[i+1:]
	}
	return host
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Port:      getenv("PORT", "8080"),
		SMTPport:  getenv("SMTP_PORT", "2525"),
		JWTSecret: getenv("JWT_SECRET", "dev-secret-change-me-32chars!!"),
		DBPath:    getenv("DB_PATH", "./mail.db"),
		RelayHost: os.Getenv("SMTP_RELAY_HOST"),
		RelayPort: getenv("SMTP_RELAY_PORT", "587"),
		RelayUser: os.Getenv("SMTP_RELAY_USER"),
		RelayPass: os.Getenv("SMTP_RELAY_PASS"),
		RelayFrom:     getenv("SMTP_RELAY_FROM", "noreply@example.com"),
		SubmitPort:    getenv("SUBMIT_PORT", "587"),
		SubmitTLSPort: os.Getenv("SUBMIT_TLS_PORT"),
		Pop3Port:      getenv("POP3_PORT", "110"),
		Pop3TLSPort:   os.Getenv("POP3_TLS_PORT"),
		ImapPort:      getenv("IMAP_PORT", "143"),
		ImapTLSPort:   os.Getenv("IMAP_TLS_PORT"),
		DKIMDomain:    dkimDomain(getenv("DKIM_DOMAIN", ""), getenv("MAIL_HOST", "mail.example.com")),
		DKIMSelector:  getenv("DKIM_SELECTOR", "dkim"),
		DKIMKey:       os.Getenv("DKIM_KEY"),
		TLSCert:       os.Getenv("TLS_CERT"),
		TLSKey:        os.Getenv("TLS_KEY"),
		Host:          getenv("MAIL_HOST", "mail.example.com"),
		AdminEmails:   os.Getenv("ADMIN_EMAILS"),
	}
}
