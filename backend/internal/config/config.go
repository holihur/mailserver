package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port            string
	SMTPport        string // 2525 本地入站（联调用）
	SubmitPort      string // 587 客户端提交（AUTH 必需）
	SubmitTLSPort   string // 465 隐式 TLS，为空则不监听
	Pop3Port        string // 110 客户端取信
	Pop3TLSPort     string // 995 隐式 TLS，为空则不监听
	ImapPort        string // 143 客户端同步
	ImapTLSPort     string // 993 隐式 TLS，为空则不监听
	ManageSievePort string // 4190 ManageSieve（Sieve 脚本管理）
	DKIMDomain      string // DKIM 签名域，默认 MAIL_HOST 去首段
	DKIMSelector    string
	DKIMKey         string // DKIM RSA 私钥 pem，为空则不签名
	TLSCert         string // 证书 pem，为空则 587/110 明文可用
	TLSKey          string
	Host            string // 本机邮件域名，用于 greeting/Message-ID
	AdminEmails     string // ADMIN_EMAILS 逗号分隔，命中即管理员（兜底提权）
	JWTSecret       string
	DatabaseURL     string // PostgreSQL DSN（必填）
	RedisURL        string // Redis DSN（限流用）
	DataDir         string // 数据目录（zones.json、证书等）
	CertDir         string // 证书 / ACME 缓存目录
	DNSAddr         string // 内置权威 DNS 监听地址（:53）；off 则禁用
	NSHost          string // NS 记录主机名，如 ns1.example.com.
	RelayHost       string // 25 被封时的外发中继（587+STARTTLS）
	RelayPort       string
	RelayUser       string
	RelayPass       string
	RelayFrom       string
	MaxMessageMB    int    // 单封邮件大小上限（MB，入站/提交）
	DMARCEnforce    string // DMARC 执行：""=跟随域策略；none/quarantine/reject 覆盖
	MetricsToken    string // METRICS_TOKEN：/metrics 鉴权（空=仅本机）
	SendDailyLimit  int    // 每用户每日发信上限（0=不限）
	SendPerMinute   int    // 每用户每分钟发信上限（0=不限）
	MtaStsMode      string // MTA-STS 模式：none（默认）| testing | enforce
	DANEEnable      bool   // 出站 DANE 验证：对方 TLSA + DNSSEC 通过时强制证书匹配
	DANEResolver    string // DANE 用的递归解析器 host:port（空=系统 resolv.conf）
	DNSSECEnable    bool   // 启用 DNSSEC 签名（密钥存 DATA_DIR/dnssec）
	DNSSECNSEC3     bool   // 使用 NSEC3（默认 NSEC）
	BackupDir       string // 定时备份目录（空=不启用）
	BackupInterval  int    // 备份间隔（小时）
	BackupKeep      int    // 保留份数
	BackupHook      string // 备份后执行的 shell 命令（如 rclone 上传），可用 $BACKUP_FILE
	// 日志
	LogFile       string // 日志文件路径；off/-/stdout 则仅输出 stdout
	LogMaxMB      int    // 单个日志文件大小上限（MB）
	LogMaxBackups int    // 保留的旧日志文件个数
	LogMaxAgeDays int    // 旧日志文件保留天数
	LogCompress   bool   // 是否 gzip 压缩旧日志
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

func atoiDefault(s string, def int) int {
	if strings.TrimSpace(s) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return def
	}
	return n
}

func isFalse(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

func Load() Config {
	dataDir := getenv("DATA_DIR", "./data")
	dbURL := os.Getenv("DATABASE_URL")
	certDir := os.Getenv("CERT_DIR")
	if certDir == "" {
		certDir = filepath.Join(dataDir, "certs")
	}
	// 日志文件：默认为 DATA_DIR/logs/mailserver.log；LOG_FILE=off/-/stdout 可关闭文件输出
	logFile := os.Getenv("LOG_FILE")
	if logFile == "" {
		logFile = filepath.Join(dataDir, "logs", "mailserver.log")
	}
	switch strings.ToLower(strings.TrimSpace(logFile)) {
	case "off", "-", "stdout", "none":
		logFile = ""
	}
	return Config{
		Port:            getenv("PORT", "8080"),
		SMTPport:        getenv("SMTP_PORT", "2525"),
		JWTSecret:       getenv("JWT_SECRET", "dev-secret-change-me-32chars!!"),
		DatabaseURL:     dbURL,
		RedisURL:        os.Getenv("REDIS_URL"),
		DataDir:         dataDir,
		CertDir:         certDir,
		DNSAddr:         getenv("DNS_ADDR", ":53"),
		NSHost:          os.Getenv("NS_HOST"),
		RelayHost:       os.Getenv("SMTP_RELAY_HOST"),
		RelayPort:       getenv("SMTP_RELAY_PORT", "587"),
		RelayUser:       os.Getenv("SMTP_RELAY_USER"),
		RelayPass:       os.Getenv("SMTP_RELAY_PASS"),
		RelayFrom:       getenv("SMTP_RELAY_FROM", "noreply@example.com"),
		MaxMessageMB:    atoiDefault(os.Getenv("MAX_MESSAGE_MB"), 25),
		DMARCEnforce:    strings.ToLower(strings.TrimSpace(getenv("DMARC_ENFORCE", "none"))),
		MetricsToken:    os.Getenv("METRICS_TOKEN"),
		SendDailyLimit:  atoiDefault(os.Getenv("SEND_DAILY_LIMIT"), 500),
		SendPerMinute:   atoiDefault(os.Getenv("SEND_PER_MINUTE"), 20),
		MtaStsMode:      strings.ToLower(strings.TrimSpace(getenv("MTA_STS_MODE", "none"))),
		DANEEnable:      getenv("DANE_ENABLE", "0") == "1",
		DANEResolver:    os.Getenv("DANE_RESOLVER"),
		DNSSECEnable:    getenv("DNSSEC_ENABLE", "0") == "1",
		DNSSECNSEC3:     getenv("DNSSEC_NSEC3", "0") == "1",
		BackupDir:       os.Getenv("BACKUP_DIR"),
		BackupInterval:  atoiDefault(os.Getenv("BACKUP_INTERVAL_HOURS"), 24),
		BackupKeep:      atoiDefault(os.Getenv("BACKUP_KEEP"), 7),
		BackupHook:      os.Getenv("BACKUP_HOOK"),
		SubmitPort:      getenv("SUBMIT_PORT", "587"),
		SubmitTLSPort:   os.Getenv("SUBMIT_TLS_PORT"),
		Pop3Port:        getenv("POP3_PORT", "110"),
		Pop3TLSPort:     os.Getenv("POP3_TLS_PORT"),
		ImapPort:        getenv("IMAP_PORT", "143"),
		ImapTLSPort:     os.Getenv("IMAP_TLS_PORT"),
		ManageSievePort: getenv("MANAGESIEVE_PORT", "4190"),
		DKIMDomain:      dkimDomain(getenv("DKIM_DOMAIN", ""), getenv("MAIL_HOST", "mail.example.com")),
		DKIMSelector:    getenv("DKIM_SELECTOR", "dkim"),
		DKIMKey:         os.Getenv("DKIM_KEY"),
		TLSCert:         os.Getenv("TLS_CERT"),
		TLSKey:          os.Getenv("TLS_KEY"),
		Host:            getenv("MAIL_HOST", "mail.example.com"),
		AdminEmails:     os.Getenv("ADMIN_EMAILS"),
		LogFile:         logFile,
		LogMaxMB:        atoiDefault(os.Getenv("LOG_MAX_MB"), 50),
		LogMaxBackups:   atoiDefault(os.Getenv("LOG_MAX_BACKUPS"), 5),
		LogMaxAgeDays:   atoiDefault(os.Getenv("LOG_MAX_AGE_DAYS"), 30),
		LogCompress:     !isFalse(os.Getenv("LOG_COMPRESS")),
	}
}
