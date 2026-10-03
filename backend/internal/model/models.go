package model

import "time"

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"uniqueIndex;size:255" json:"email"`
	Name      string    `gorm:"size:100" json:"name"`
	PassHash  string    `gorm:"size:255" json:"-"`
	Admin     bool      `json:"admin"`    // 管理员：可进 /api/admin 管理后台
	Disabled  bool      `json:"disabled"` // 禁用：Web/API/收发信全部拒绝
	CreatedAt time.Time `json:"created_at"`
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

// folder: inbox / sent / draft / trash
// Relayed: 发件队列状态（sent 文件夹有效），25 被封时走中继投递
type Mail struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"-"`
	From      string    `gorm:"size:255" json:"from"`
	To        string    `gorm:"size:255" json:"to"`
	Subject   string    `gorm:"size:500" json:"subject"`
	Body      string    `gorm:"type:text" json:"body"`
	Folder    string    `gorm:"size:20;index" json:"folder"`
	Read      bool      `json:"read"`
	Starred   bool      `json:"starred"`
	Relayed   bool      `json:"relayed"`
	RelayErr  string    `gorm:"size:500" json:"relay_err"`
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
}
