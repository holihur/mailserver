package queue

// 发件队列：25 出站被封是常态，所以不直连对方 MX，只做两件事：
// 1) 本站用户之间 -> 直接投对方 inbox（秒到，不经过公网）；
// 2) 站外地址 -> 经配置的 RelayHost:RelayPort（587+STARTTLS）发出，配了 DKIM_KEY 则自签名。
// 无中继时站外信会标记 relay_err，前端 /setup 页可见，不静默丢信。

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"mailserver/internal/dkim"
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
	// 1) 本站：直接 inbox
	var u model.User
	if err := db.Where("email = ?", m.To).First(&u).Error; err == nil {
		db.Create(&model.Mail{UserID: u.ID, From: m.From, To: m.To, Subject: m.Subject, Body: m.Body, Folder: "inbox"})
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
	if err := sendSMTP(c, m.From, []string{m.To}, msg); err != nil {
		if c.From != "" {
			if err2 := sendSMTP(c, c.From, []string{m.To}, msg); err2 == nil {
				db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": ""})
				return
			}
		}
		db.Model(m).Updates(map[string]any{"attempts": m.Attempts + 1, "relay_err": trimErr(err)})
		log.Printf("queue: mail %d to %s fail: %v", m.ID, m.To, err)
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
	if host == "" {
		host = "mailserver.local"
	}
	subj := m.Subject
	if subj == "" {
		subj = "(无主题)"
	}
	headers := [][2]string{
		{"From", m.From},
		{"To", m.To},
		{"Subject", subj},
		{"Date", m.CreatedAt.Format(time.RFC1123Z)},
		{"Message-ID", fmt.Sprintf("<%d.%d@%s>", m.ID, time.Now().UnixNano(), host)},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/plain; charset=utf-8"},
	}
	var b strings.Builder
	for _, h := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", h[0], h[1])
	}
	if signer != nil && signer.Match(m.From) {
		if sig := signer.Sign(headers, m.Body); sig != "" {
			b.WriteString(sig + "\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return []byte(b.String())
}

func trimErr(err error) string {
	s := err.Error()
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
