package model

import "time"

type User struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Email       string    `gorm:"uniqueIndex;size:255" json:"email"`
	Name        string    `gorm:"size:100" json:"name"`
	PassHash    string    `gorm:"size:255" json:"-"`
	Admin       bool      `json:"admin"`             // 管理员：可进 /api/admin 管理后台
	Disabled    bool      `json:"disabled"`          // 禁用：Web/API/收发信全部拒绝
	TOTPSecret  string    `gorm:"size:255" json:"-"` // AES-GCM 加密的 TOTP 密钥
	TOTPEnabled bool      `json:"totp_enabled"`      // 登录是否要求动态验证码
	CreatedAt   time.Time `json:"created_at"`
}

// MailToken 邮件客户端专用令牌（PAT，应用专用密码）。
// IMAP/POP3/SMTP 客户端用它代替网页登录密码；明文只在创建时返回一次，
// 库中仅存 sha256（令牌为 256bit 随机值，无需慢哈希），Prefix 供界面展示。
type MailToken struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index" json:"user_id"`
	Name      string     `gorm:"size:120" json:"name"`
	Prefix    string     `gorm:"size:20;index" json:"prefix"`
	Hash      string     `gorm:"size:64;uniqueIndex" json:"-"`
	LastUsed  *time.Time `json:"last_used"`
	CreatedAt time.Time  `json:"created_at"`
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
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"-"`
	From        string    `gorm:"size:255" json:"from"`
	To          string    `gorm:"size:255" json:"to"`
	Cc          string    `gorm:"size:255" json:"cc"`
	Bcc         string    `gorm:"size:255" json:"bcc"`
	Subject     string    `gorm:"size:500" json:"subject"`
	Body        string    `gorm:"type:text" json:"body"`
	Attachments string    `gorm:"type:text" json:"attachments"` // JSON: [{name,type,data(base64),size}]
	Folder      string    `gorm:"size:20;index" json:"folder"`
	Read        bool      `json:"read"`
	Starred     bool      `json:"starred"`
	Relayed     bool      `json:"relayed"`
	RelayErr    string    `gorm:"size:500" json:"relay_err"`
	Attempts    int       `json:"attempts"`
	CreatedAt   time.Time `json:"created_at"`
}

// MailRule 基于 CEL 的收信规则：表达式命中后把邮件投递到指定文件夹（默认 trash）。
// UserID=0 为整站规则（仅管理员维护），>0 为用户级规则。优先级大的先匹配。
type MailRule struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	Name       string    `gorm:"size:120" json:"name"`
	Enabled    bool      `json:"enabled"`
	Priority   int       `json:"priority"`
	Expression string    `gorm:"type:text" json:"expression"` // CEL，返回 bool
	Action     string    `gorm:"size:20" json:"action"`       // trash | move
	Folder     string    `gorm:"size:30" json:"folder"`       // action=move 时的目标文件夹
	CreatedAt  time.Time `json:"created_at"`
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
