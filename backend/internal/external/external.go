// Package external 第三方邮箱账号：通过 IMAP 拉取来信，并提供该账号的 SMTP 发信配置。
package external

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
	"time"

	"mailserver/internal/htmlsanitize"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/secret"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Pass 解密存储的密码。
func Pass(enc string) string {
	if enc == "" {
		return ""
	}
	if b, err := secret.Decrypt(enc); err == nil {
		return string(b)
	}
	return ""
}

// Find 按发件地址查找用户绑定的第三方账号（用于「以该身份发信」）。
func Find(db *gorm.DB, userID uint, from string) *model.ExternalAccount {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" {
		return nil
	}
	var a model.ExternalAccount
	if err := db.Where("user_id = ? AND LOWER(email) = ?", userID, from).First(&a).Error; err != nil {
		return nil
	}
	return &a
}

// Relay 把第三方账号的 SMTP 配置转换为外发中继配置。
func Relay(a *model.ExternalAccount) runtimecfg.Relay {
	port := strings.TrimSpace(a.SMTPPort)
	if port == "" {
		if a.SMTPSSL {
			port = "465"
		} else {
			port = "587"
		}
	}
	host := strings.TrimSpace(a.SMTPHost)
	return runtimecfg.Relay{
		Host: host, Port: port, User: a.SMTPUser, Pass: Pass(a.SMTPPass),
		From: a.Email, Name: host,
	}
}

func portOf(p string, ssl bool) string {
	if strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p)
	}
	if ssl {
		return "993"
	}
	return "143"
}

// Sync 连接第三方 IMAP，拉取未读邮件存入用户收件箱，并在服务端标记为已读。返回导入封数。
func Sync(db *gorm.DB, a *model.ExternalAccount) (int, error) {
	c, err := connect(a)
	if err != nil {
		return 0, err
	}
	defer func() { _ = c.Logout() }()
	crit := imap.NewSearchCriteria()
	crit.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(crit)
	if err != nil {
		return 0, err
	}
	return fetchCreate(db, a, c, uids, true)
}

// SyncHistory 回填历史邮件（含已读），跳过已同步 UID，每次最多 limit 封；
// 不修改服务端已读状态。返回导入封数。
func SyncHistory(db *gorm.DB, a *model.ExternalAccount, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	c, err := connect(a)
	if err != nil {
		return 0, err
	}
	defer func() { _ = c.Logout() }()
	all, err := c.UidSearch(imap.NewSearchCriteria())
	if err != nil {
		return 0, err
	}
	synced := syncedUIDs(db, a.ID)
	todo := make([]uint32, 0, limit)
	for _, u := range all { // UidSearch 升序（最旧在前）
		if !synced[u] {
			todo = append(todo, u)
			if len(todo) >= limit {
				break
			}
		}
	}
	return fetchCreate(db, a, c, todo, false)
}

// connect 连接并登录第三方 IMAP，选中 INBOX。
func connect(a *model.ExternalAccount) (*client.Client, error) {
	addr := net.JoinHostPort(a.IMAPHost, portOf(a.IMAPPort, a.IMAPSSL))
	var c *client.Client
	var err error
	if a.IMAPSSL {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: a.IMAPHost, MinVersion: tls.VersionTLS12})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, err
	}
	if err := c.Login(a.IMAPUser, Pass(a.IMAPPass)); err != nil {
		_ = c.Logout()
		return nil, err
	}
	if _, err := c.Select("INBOX", false); err != nil {
		_ = c.Logout()
		return nil, err
	}
	return c, nil
}

func syncedUIDs(db *gorm.DB, accountID uint) map[uint32]bool {
	var us []uint32
	db.Model(&model.ExternalSync{}).Where("account_id = ?", accountID).Pluck("uid", &us)
	m := make(map[uint32]bool, len(us))
	for _, u := range us {
		m[u] = true
	}
	return m
}

// fetchCreate 拉取指定 UID 并写入收件箱，记录已同步 UID；markSeen=true 时在服务端标记已读。
func fetchCreate(db *gorm.DB, a *model.ExternalAccount, c *client.Client, uids []uint32, markSeen bool) (int, error) {
	if len(uids) == 0 {
		return 0, nil
	}
	synced := syncedUIDs(db, a.ID)
	seqset := new(imap.SeqSet)
	added := 0
	for _, u := range uids {
		if !synced[u] {
			seqset.AddNum(u)
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags, section.FetchItem()}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seqset, items, messages) }()

	n := 0
	for msg := range messages {
		r := msg.GetBody(section)
		if r == nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(r, 16<<20))
		from := ""
		if msg.Envelope != nil && len(msg.Envelope.From) > 0 {
			from = msg.Envelope.From[0].Address()
		}
		subject, body, htmlBody, atts := message.ParseInbound(string(raw))
		read := markSeen || hasFlag(msg.Flags, imap.SeenFlag)
		db.Create(&model.Mail{UserID: a.UserID, From: from, To: a.Email,
			Subject: subject, Body: body, BodyHTML: htmlsanitize.Sanitize(htmlBody), Attachments: atts, Folder: "inbox", Read: read})
		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ExternalSync{AccountID: a.ID, UID: msg.Uid})
		n++
		if markSeen {
			s := new(imap.SeqSet)
			s.AddNum(msg.Uid)
			_ = c.UidStore(s, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.SeenFlag}, nil)
		}
	}
	return n, <-done
}

func hasFlag(flags []string, f string) bool {
	for _, x := range flags {
		if x == f {
			return true
		}
	}
	return false
}

// Start 周期性同步所有启用的第三方账号。
func Start(db *gorm.DB) {
	run := func() {
		var accs []model.ExternalAccount
		db.Where("enabled = ?", true).Find(&accs)
		for i := range accs {
			_, err := Sync(db, &accs[i])
			if err == nil && accs[i].SyncHistory {
				// 渐进回填历史（每次限流 SyncLimit），直至追平
				if _, e2 := SyncHistory(db, &accs[i], accs[i].SyncLimit); e2 != nil {
					err = e2
				}
			}
			upd := map[string]any{"last_sync": time.Now()}
			if err != nil {
				upd["last_error"] = trim(err.Error())
			} else {
				upd["last_error"] = ""
			}
			db.Model(&model.ExternalAccount{}).Where("id = ?", accs[i].ID).Updates(upd)
		}
	}
	run()
	for range time.Tick(5 * time.Minute) {
		run()
	}
}

func trim(s string) string {
	if len(s) > 480 {
		return s[:480]
	}
	return s
}
