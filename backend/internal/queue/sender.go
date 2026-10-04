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
	"sort"
	"strings"
	"time"

	localdeliver "mailserver/internal/deliver"
	"mailserver/internal/dkim"
	"mailserver/internal/external"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/route"
	"mailserver/internal/runtimecfg"

	"gorm.io/gorm"
)

const MaxAttempts = 8

// LoadRoutes 读取启用的邮件路由（供 asynq worker 使用）。
func LoadRoutes(db *gorm.DB) []model.MailRoute {
	var routes []model.MailRoute
	db.Where("enabled = ?", true).Find(&routes)
	return routes
}

func Deliver(db *gorm.DB, c runtimecfg.Relay, signer *dkim.Signer, routes []model.MailRoute, m *model.Mail) {
	db.Model(m).Update("status", "sending")
	// 以第三方账号身份发信时，改用该账号的 SMTP 配置
	if ext := external.Find(db, m.UserID, m.From); ext != nil {
		c = external.Relay(ext)
	}
	recipients := message.Recipients(m)
	if len(recipients) == 0 {
		db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": "无收件人", "status": "sent"})
		return
	}

	// 1) 本站收件人直接投递到 inbox；其余需外发
	var external []string
	for _, rcpt := range recipients {
		var u model.User
		if err := db.Where("LOWER(email) = ?", strings.ToLower(rcpt)).First(&u).Error; err == nil {
			localdeliver.ToUser(db, &u, m.From, rcpt, m.Cc, m.Bcc, m.Subject, m.Body, m.BodyHTML, m.Attachments)
		} else {
			external = append(external, rcpt)
		}
	}
	if len(external) == 0 {
		db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": "", "status": "sent"})
		return
	}

	// 2) 站外：按域名分组，各自解析路由（relay/direct/discard）
	msg := buildMsg(c.Name, m, signer)
	byDomain := map[string][]string{}
	var order []string
	for _, rcpt := range external {
		at := strings.LastIndex(rcpt, "@")
		if at < 0 {
			continue
		}
		d := strings.ToLower(rcpt[at+1:])
		if _, ok := byDomain[d]; !ok {
			order = append(order, d)
		}
		byDomain[d] = append(byDomain[d], rcpt)
	}
	var firstErr error
	for _, dom := range order {
		rcpts := byDomain[dom]
		rt := route.Match(routes, dom)
		switch {
		case rt != nil && rt.Action == "discard":
			log.Printf("queue: mail %d to %s discarded by route", m.ID, dom)
		case rt != nil && rt.Action == "direct":
			if err := sendDirect(m.From, rcpts, msg); err != nil && firstErr == nil {
				firstErr = err
			}
		case rt != nil && rt.Action == "relay":
			rc := route.Relay(rt)
			if err := sendSMTP(rc, m.From, rcpts, msg); err != nil {
				if rc.From != "" {
					if err2 := sendSMTP(rc, rc.From, rcpts, msg); err2 == nil {
						continue
					}
				}
				if firstErr == nil {
					firstErr = err
				}
			}
		default:
			// 无匹配路由：用全局中继 / 直连
			if c.Host == "" {
				if c.Direct {
					if err := sendDirect(m.From, rcpts, msg); err != nil && firstErr == nil {
						firstErr = err
					}
				} else if firstErr == nil {
					firstErr = fmt.Errorf("无外发中继：25 出站被封，请配 SMTP_RELAY_HOST，或在后台开启「直连对方 MX」")
				}
			} else if err := sendSMTP(c, m.From, rcpts, msg); err != nil {
				if c.From != "" {
					if err2 := sendSMTP(c, c.From, rcpts, msg); err2 == nil {
						continue
					}
				}
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	if firstErr != nil {
		status := "queued"
		if m.Attempts+1 >= MaxAttempts {
			status = "failed"
		}
		db.Model(m).Updates(map[string]any{"attempts": m.Attempts + 1, "relay_err": trimErr(firstErr), "status": status})
		log.Printf("queue: mail %d external fail: %v", m.ID, firstErr)
		return
	}
	db.Model(m).Updates(map[string]any{"relayed": true, "relay_err": "", "status": "sent"})
}

// sendSMTP 通过中继发信；465 用隐式 TLS，25/587 用 STARTTLS。
func sendSMTP(c runtimecfg.Relay, from string, to []string, msg []byte) error {
	host := strings.TrimSpace(c.Host)
	port := strings.TrimSpace(c.Port)
	if port == "" {
		port = "587"
	}
	return smtpSession(host, port, c.User, c.Pass, c.Insecure, from, to, msg)
}

func smtpSession(host, port, user, pass string, insecure bool, from string, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 15*time.Second)
	if err != nil {
		return err
	}
	if port == "465" || port == "8465" {
		conn = tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure})
	}
	cl, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer cl.Close()
	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure}); err != nil {
			return err
		}
	}
	if user != "" {
		if ok, _ := cl.Extension("AUTH"); ok {
			if err := cl.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
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

// sendDirect 无中继时直连对方 MX:25 投递（需 25 出站放行）。
func sendDirect(from string, to []string, msg []byte) error {
	byDomain := map[string][]string{}
	for _, rcpt := range to {
		at := strings.LastIndex(rcpt, "@")
		if at < 0 {
			return fmt.Errorf("非法收件人: %s", rcpt)
		}
		d := strings.ToLower(rcpt[at+1:])
		byDomain[d] = append(byDomain[d], rcpt)
	}
	var firstErr error
	for dom, rcpts := range byDomain {
		delivered := false
		for _, h := range mxHosts(dom) {
			if err := smtpSession(h, "25", "", "", false, from, rcpts, msg); err == nil {
				delivered = true
				break
			} else if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", h, err)
			}
		}
		if !delivered && firstErr == nil {
			firstErr = fmt.Errorf("投递到 %s 失败", dom)
		}
	}
	return firstErr
}

// mxHosts 返回按优先级排序的 MX 主机；无 MX 时按 RFC 回退到域本身。
func mxHosts(dom string) []string {
	mxs, err := net.LookupMX(dom)
	if err != nil || len(mxs) == 0 {
		return []string{dom}
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	out := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		out = append(out, strings.TrimSuffix(mx.Host, "."))
	}
	return out
}

// 组装 RFC5322；发件域==签名域且配了私钥时加 DKIM-Signature（签的就是实际发出的头）
func buildMsg(host string, m *model.Mail, signer *dkim.Signer) []byte {
	if m.IsMDN {
		return message.BuildMDN(host, m)
	}
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
