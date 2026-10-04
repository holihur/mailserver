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

// Sync 连接第三方 IMAP，拉取未读邮件存入用户收件箱，并在服务端标记为已读。
func Sync(db *gorm.DB, a *model.ExternalAccount) error {
	addr := net.JoinHostPort(a.IMAPHost, portOf(a.IMAPPort, a.IMAPSSL))
	var c *client.Client
	var err error
	if a.IMAPSSL {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: a.IMAPHost, MinVersion: tls.VersionTLS12})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return err
	}
	defer c.Logout()
	if err := c.Login(a.IMAPUser, Pass(a.IMAPPass)); err != nil {
		return err
	}
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return err
	}
	if mbox.Messages == 0 {
		return nil
	}
	crit := imap.NewSearchCriteria()
	crit.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(crit)
	if err != nil {
		return err
	}
	if len(uids) == 0 {
		return nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, section.FetchItem()}
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seqset, items, messages) }()

	for msg := range messages {
		r := msg.GetBody(section)
		if r == nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(r, 8<<20))
		from := ""
		if msg.Envelope != nil && len(msg.Envelope.From) > 0 {
			from = msg.Envelope.From[0].Address()
		}
		subject, body, htmlBody, atts := message.ParseInbound(string(raw))
		db.Create(&model.Mail{UserID: a.UserID, From: from, To: a.Email,
			Subject: subject, Body: body, BodyHTML: htmlsanitize.Sanitize(htmlBody), Attachments: atts, Folder: "inbox"})
		// 服务端标记已读，避免下次重复拉取
		s := new(imap.SeqSet)
		s.AddNum(msg.Uid)
		_ = c.UidStore(s, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.SeenFlag}, nil)
	}
	return <-done
}

// Start 周期性同步所有启用的第三方账号。
func Start(db *gorm.DB) {
	run := func() {
		var accs []model.ExternalAccount
		db.Where("enabled = ?", true).Find(&accs)
		for i := range accs {
			err := Sync(db, &accs[i])
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
