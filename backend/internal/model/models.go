package model

import "time"

type User struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Email       string `gorm:"uniqueIndex;size:255" json:"email"`
	Name        string `gorm:"size:100" json:"name"`
	Signature   string `gorm:"size:1000" json:"signature"` // 邮件签名
	QuotaMB     int    `json:"quota_mb"`                   // 存储配额（MB），0=不限
	QuotaUsed   int64  `gorm:"-" json:"quota_used"`        // 已用字节（仅展示）
	PassHash    string `gorm:"size:255" json:"-"`
	Admin       bool   `json:"admin"`             // 管理员：可进 /api/admin 管理后台
	Disabled    bool   `json:"disabled"`          // 禁用：Web/API/收发信全部拒绝
	TOTPSecret  string `gorm:"size:255" json:"-"` // AES-GCM 加密的 TOTP 密钥
	TOTPEnabled bool   `json:"totp_enabled"`      // 登录是否要求动态验证码
	// MustChangePassword 管理员重置密码后置 true，强制下次登录改密
	MustChangePassword bool `json:"must_change_password"`
	// TokenVersion 令牌版本：改密/重置/关闭 TOTP 时自增，旧 access token 立即失效。
	TokenVersion int       `gorm:"not null;default:0" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// MailToken 邮件客户端专用令牌（PAT，应用专用密码）。
// IMAP/POP3/SMTP 客户端用它代替网页登录密码；明文只在创建时返回一次，
// 库中仅存 sha256（令牌为 256bit 随机值，无需慢哈希），Prefix 供界面展示。
type MailToken struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index" json:"user_id"`
	Name         string     `gorm:"size:120" json:"name"`
	Prefix       string     `gorm:"size:20;index" json:"prefix"`
	Hash         string     `gorm:"size:64;uniqueIndex" json:"-"`
	AllowedCIDRs string     `gorm:"size:500" json:"allowed_cidrs"` // 允许使用的来源 CIDR，逗号分隔；空=不限
	Scopes       string     `gorm:"size:200" json:"scopes"`        // 授权范围（imap,pop3,smtp,jmap,sieve,mcp），逗号分隔；空=不限
	LastUsed     *time.Time `json:"last_used"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ScheduledMail 定时 / 周期性发送的邮件：到期后由队列生成一封普通 sent 邮件发出。
// Repeat 为空表示只发一次；daily/weekly/monthly 表示循环发送。
type ScheduledMail struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"index" json:"user_id"`
	From        string     `gorm:"size:255" json:"from"`
	To          string     `gorm:"size:255" json:"to"`
	Cc          string     `gorm:"size:255" json:"cc"`
	Bcc         string     `gorm:"size:255" json:"bcc"`
	Subject     string     `gorm:"size:500" json:"subject"`
	Body        string     `gorm:"type:text" json:"body"`
	Attachments string     `gorm:"type:text" json:"attachments"`
	ReceiptTo   string     `gorm:"size:255" json:"receipt_to"`
	SendAt      time.Time  `gorm:"index" json:"send_at"`
	Repeat      string     `gorm:"size:20" json:"repeat"` // "" | daily | weekly | monthly
	Enabled     bool       `json:"enabled"`
	LastSent    *time.Time `json:"last_sent"`
	LastError   string     `gorm:"size:500" json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Session 网页登录会话（逐会话踢出）。
type Session struct {
	JTI       string    `gorm:"primaryKey;size:64" json:"jti"`
	UserID    uint      `gorm:"index" json:"user_id"`
	IP        string    `gorm:"size:64" json:"ip"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LoginEvent 登录历史（成功/失败），用于用户查看与异常告警。
type LoginEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Email     string    `gorm:"size:255" json:"email"`
	IP        string    `gorm:"size:64" json:"ip"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	Success   bool      `json:"success"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog 管理员高危操作审计日志（脱敏后记录）。
type AuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ActorID    uint      `gorm:"index" json:"actor_id"`
	ActorEmail string    `gorm:"size:255" json:"actor_email"`
	Action     string    `gorm:"size:20" json:"action"`  // 方法：POST/PATCH/DELETE
	Target     string    `gorm:"size:255" json:"target"` // 接口路径
	Detail     string    `gorm:"size:1000" json:"detail"`
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
}

// ---- 自托管域名/DNS ----
type Domain struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:255" json:"name"` // example.com，不带点
	CreatedAt time.Time `json:"created_at"`
}

// Name: @ / mail / dkim._domainkey，相对域名；Type: A/MX/TXT/CNAME/NS/SRV/CAA/AAAA
type DnsRecord struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	DomainID uint   `gorm:"index" json:"domain_id"`
	Name     string `gorm:"size:255;index" json:"name"`
	Type     string `gorm:"size:10;index" json:"type"`
	Value    string `gorm:"size:1024" json:"value"`
	TTL      int    `json:"ttl"`
	Prio     int    `json:"prio"` // MX/SRV 优先级，存 Value 前缀亦可，此处单独列
}

// 第三方域名服务商账号（阿里云 DNS / Cloudflare），用于一键下发邮件解析。
// Creds 存 AES-GCM 加密后的 JSON（key 由 JWT_SECRET 派生），json 不外泄。
type DnsProvider struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:120" json:"name"`
	Type      string    `gorm:"size:20;index" json:"type"` // aliyun | cloudflare
	Creds     string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// ACME/Let's Encrypt 自动签发配置（单例，通常仅一行）。
// ProviderID 指向 DnsProvider，用于 DNS-01 质询（在服务商处自动写 _acme-challenge）。
type AcmeConfig struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Domain     string    `gorm:"size:255" json:"domain"`
	ProviderID uint      `json:"provider_id"`
	Email      string    `gorm:"size:255" json:"email"`
	AutoRenew  bool      `json:"auto_renew"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Setting 运行时可改配置（管理后台写入，DB 持久化 + 内存缓存）。
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// folder: inbox / sent / draft / trash
// Relayed: 发件队列状态（sent 文件夹有效），25 被封时走中继投递
type Mail struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	UserID       uint   `gorm:"index" json:"-"`
	From         string `gorm:"size:255" json:"from"`
	EnvelopeFrom string `gorm:"size:255" json:"envelope_from"` // SRS 重写后的信封发件人（空则用 From）
	To           string `gorm:"size:255" json:"to"`
	Cc           string `gorm:"size:255" json:"cc"`
	Bcc          string `gorm:"size:255" json:"bcc"`
	Subject      string `gorm:"size:500" json:"subject"`
	Body         string `gorm:"type:text" json:"body"`
	BodyHTML     string `gorm:"type:text" json:"body_html"`   // 清洗后的 HTML 正文
	Attachments  string `gorm:"type:text" json:"attachments"` // JSON: [{name,type,data(base64),size}]
	Folder       string `gorm:"size:20;index" json:"folder"`
	Read         bool   `json:"read"`
	Starred      bool   `json:"starred"`
	Relayed      bool   `json:"relayed"`
	RelayErr     string `gorm:"size:500" json:"relay_err"`
	Status       string `gorm:"size:20" json:"status"` // sent 文件夹：queued|sending|sent|failed
	Attempts     int    `json:"attempts"`
	// 已读回执（MDN, RFC 3798）
	ReceiptTo   string     `gorm:"size:255" json:"receipt_to"`   // 非空：收到的邮件要求回执至此地址；发件时表示已请求回执
	ReceiptSent bool       `json:"receipt_sent"`                 // 收到时：已回复回执
	ReceiptRead bool       `json:"receipt_read"`                 // 发出时：已收到对方回执
	ReceiptAt   *time.Time `json:"receipt_at"`                   // 回执到达时间
	IsMDN       bool       `json:"is_mdn"`                       // 本条本身是一封已读回执
	ReceiptFor  uint       `json:"receipt_for"`                  // MDN 关联的原邮件 ID
	AuthResults string     `gorm:"size:255" json:"auth_results"` // 入站认证结果，如 "spf=pass; dkim=fail; dmarc=fail"
	CreatedAt   time.Time  `json:"created_at"`

	// 会话聚合（仅接口返回，非持久化）
	ThreadCount  int    `gorm:"-" json:"thread_count,omitempty"`
	ThreadUnread int    `gorm:"-" json:"thread_unread,omitempty"`
	ThreadIDs    []uint `gorm:"-" json:"thread_ids,omitempty"`
}

// MailRule 基于 CEL 的收信规则：表达式命中后把邮件投递到指定文件夹（默认 trash）。
// UserID=0 为整站规则（仅管理员维护），>0 为用户级规则。优先级大的先匹配。
type MailRule struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	Name       string    `gorm:"size:120" json:"name"`
	Enabled    bool      `json:"enabled"`
	Shadow     bool      `json:"shadow"` // 影子模式：只匹配不执行（不移动/转发/丢弃）
	Priority   int       `json:"priority"`
	Expression string    `gorm:"type:text" json:"expression"` // CEL，返回 bool
	Action     string    `gorm:"size:20" json:"action"`       // trash | move | forward
	Folder     string    `gorm:"size:30" json:"folder"`       // action=move 时的目标文件夹
	ForwardTo  string    `gorm:"size:1024" json:"forward_to"` // action=forward 时的转发目标（逗号分隔）
	CreatedAt  time.Time `json:"created_at"`
}

// MailFolder 用户自定义文件夹。Mail.Folder 存其键 "c<ID>"。
type MailFolder struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Name      string    `gorm:"size:120" json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SieveScript 用户的 Sieve 脚本（RFC 5228 子集）。同一用户可有多个脚本，至多一个 active。
type SieveScript struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Name      string    `gorm:"size:120" json:"name"`
	Script    string    `gorm:"type:text" json:"script"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Contact 用户联系人（通讯录）。Note 为备注，可随时修改。
type Contact struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex:idx_contact_user_email;index" json:"user_id"`
	Email     string    `gorm:"uniqueIndex:idx_contact_user_email;size:255" json:"email"`
	Name      string    `gorm:"size:120" json:"name"`
	Note      string    `gorm:"size:500" json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// MailRoute 外发邮件路由（管理员配置）：按收件人域名把邮件交给指定中继 / 直连 / 丢弃。
// Domain 支持精确域名（example.com）或后缀（.example.com 匹配子域）；Priority 大的先匹配。
type MailRoute struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Domain    string    `gorm:"size:255;index" json:"domain"`
	Action    string    `gorm:"size:20" json:"action"` // relay | direct | discard
	RelayHost string    `gorm:"size:255" json:"relay_host"`
	RelayPort string    `gorm:"size:10" json:"relay_port"`
	RelayUser string    `gorm:"size:255" json:"relay_user"`
	RelayPass string    `gorm:"size:512" json:"-"` // AES-GCM 加密
	RelayFrom string    `gorm:"size:255" json:"relay_from"`
	Insecure  bool      `json:"insecure"`
	Priority  int       `json:"priority"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// MailAlias 收件人别名 / 转发（管理员配置）：把发给 Source 的邮件投递到多个 Targets。
// Source 可为完整地址（abc@example.com）或整域 catch-all（@example.com）。
// Target 是本地用户则进其收件箱，是外部地址则自动转发（入队外发）。
type MailAlias struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Source    string    `gorm:"size:255;index" json:"source"`
	Targets   string    `gorm:"type:text" json:"targets"` // 逗号分隔的地址列表
	Keep      bool      `json:"keep"`                     // 是否同时保留原收件人
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ExternalAccount 第三方邮箱账号（用户自配）：从该账号 IMAP 收信，并可作为发件身份经其 SMTP 发信。
// 密码用 AES-GCM 加密存储。
type ExternalAccount struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index" json:"user_id"`
	Email     string     `gorm:"size:255" json:"email"`
	Name      string     `gorm:"size:120" json:"name"`
	IMAPHost  string     `gorm:"size:255" json:"imap_host"`
	IMAPPort  string     `gorm:"size:10" json:"imap_port"`
	IMAPSSL   bool       `json:"imap_ssl"`
	IMAPUser  string     `gorm:"size:255" json:"imap_user"`
	IMAPPass  string     `gorm:"size:512" json:"-"`
	SMTPHost  string     `gorm:"size:255" json:"smtp_host"`
	SMTPPort  string     `gorm:"size:10" json:"smtp_port"`
	SMTPSSL   bool       `json:"smtp_ssl"`
	SMTPUser  string     `gorm:"size:255" json:"smtp_user"`
	SMTPPass  string     `gorm:"size:512" json:"-"`
	Enabled   bool       `json:"enabled"`
	LastSync  *time.Time `json:"last_sync"`
	LastError string     `gorm:"size:500" json:"last_error"`
	CreatedAt time.Time  `json:"created_at"`
}
