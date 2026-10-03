package queue

// 发件队列：25 出站被封是常态，所以不直连对方 MX，只做两件事：
// 1) 本站用户之间 -> 直接投对方 inbox（秒到，不经过公网）；
// 2) 站外地址 -> 经配置的 RelayHost:RelayPort（587+STARTTLS）发出，配了 DKIM_KEY 则自签名。
// 无中继时站外信会标记 relay_err，前端 /setup 页可见，不静默丢信。

import (
	"crypto/tls"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"mailserver/internal/dkim"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"

	"gorm.io/gorm"
)

const maxAttempts = 8

func Start(db *gorm.DB, rt *runtimecfg.Store) {
	run := func() {
		c := rt.Relay()
		signer := rt.Signer()
		var mails []model.Mail
		db.Where("folder = ? AND relayed = ? AND attempts < ?", "sent", false, maxAttempts).
			Order("id").Limit(20).Find(&mails)
		for i := range mails {
			deliver(db, c, signer, &mails[i])
		}
	}
	run()
	for range time.Tick(20 * time.Second) {
		run()
	}
}

func deliver(db *gorm.DB, c runtimecfg.Relay, signer *dkim.Signer, m *model.Mail) {
	recipients := message.Recipients(m)
	if len(recipients) == 0 {
		db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": "无收件人"})
		return
	}

	// 1) 本站收件人直接投递到 inbox；其余需外发
	var external []string
	for _, rcpt := range recipients {
		var u model.User
		if err := db.Where("LOWER(email) = ?", strings.ToLower(rcpt)).First(&u).Error; err == nil {
			db.Create(&model.Mail{UserID: u.ID, From: m.From, To: rcpt, Cc: m.Cc,
				Subject: m.Subject, Body: m.Body, Attachments: m.Attachments, Folder: "inbox"})
		} else {
			external = append(external, rcpt)
		}
	}
	if len(external) == 0 {
		db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": ""})
		return
	}

	// 2) 站外：必须有中继（25 被封，直投必失败，不尝试）
	if c.Host == "" {
		db.Model(m).Updates(map[string]any{
			"attempts":  m.Attempts + 1,
			"relay_err": "无外发中继：25 出站被封，请配 SMTP_RELAY_HOST（见 MAIL_CLIENTS.md）",
		})
		return
	}
	// 默认 envelope-from 用本人地址；若中继强制要求认证账号一致，失败后会自动用 RelayFrom 重试一次
	msg := buildMsg(c.Name, m, signer)
	if err := sendSMTP(c, m.From, external, msg); err != nil {
		if c.From != "" {
			if err2 := sendSMTP(c, c.From, external, msg); err2 == nil {
				db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": ""})
				return
			}
		}
		db.Model(m).Updates(map[string]any{"attempts": m.Attempts + 1, "relay_err": trimErr(err)})
		log.Printf("queue: mail %d to %v fail: %v", m.ID, external, err)
		return
	}
	db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": ""})
}

// sendSMTP 通过中继发信；465 用隐式 TLS，25/587 用 STARTTLS。
func sendSMTP(c runtimecfg.Relay, from string, to []string, msg []byte) error {
	host := strings.TrimSpace(c.Host)
	port := strings.TrimSpace(c.Port)
	if port == "" {
		port = "587"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 15*time.Second)
	if err != nil {
		return err
	}
	if port == "465" || port == "8465" {
		conn = tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.Insecure})
	}
	cl, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer cl.Close()
	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.Insecure}); err != nil {
			return err
		}
	}
	if c.User != "" {
		if ok, _ := cl.Extension("AUTH"); ok {
			if err := cl.Auth(smtp.PlainAuth("", c.User, c.Pass, host)); err != nil {
				return err
			}
		}
	}
	if err := cl.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := cl.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

// 组装 RFC5322；发件域==签名域且配了私钥时加 DKIM-Signature（签的就是实际发出的头）
func buildMsg(host string, m *model.Mail, signer *dkim.Signer) []byte {
	hdrs, body := message.Parts(host, m)
	var extra []string
	if signer != nil && signer.Match(m.From) {
		if sig := signer.Sign(hdrs, body); sig != "" {
			extra = append(extra, sig)
		}
	}
	return message.Serialize(hdrs, body, extra...)
}

func trimErr(err error) string {
	s := err.Error()
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
