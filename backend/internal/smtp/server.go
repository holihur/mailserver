package smtp

// 极简入站 SMTP（:2525），低内存：逐连接 goroutine，逐行解析，只收 DATA 存库。
// 生产收信请用 Postfix/Exim，此仅供本地联调。

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"mailserver/internal/alias"
	"mailserver/internal/deliver"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/quota"

	"gorm.io/gorm"
)

func Serve(addr string, db *gorm.DB) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("smtp listen fail:", err)
		return
	}
	fmt.Println("smtp inbound on", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, db)
	}
}

func handle(c net.Conn, db *gorm.DB) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	reply := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
	reply("220 mailserver ESMTP ready")

	var from, to string
	var data strings.Builder
	inData := false

	for {
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Minute))
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				saveMail(db, from, to, data.String())
				data.Reset()
				reply("250 OK: queued")
				continue
			}
			data.WriteString(line + "\n")
			if data.Len() > 1<<20 { // 1MB 截断，防爆内存
				inData = false
				reply("552 too large")
			}
			continue
		}
		up := strings.ToUpper(line)
		if len(line) > 4096 {
			reply("500 line too long")
			return
		}
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			reply("250-Hello\r\n250 8BITMIME")
		case strings.HasPrefix(up, "MAIL FROM:"):
			from = extractAddr(line)
			reply("250 OK")
		case strings.HasPrefix(up, "RCPT TO:"):
			addr := extractAddr(line)
			if !recipientExists(db, addr) {
				reply("550 5.1.1 User unknown")
				continue
			}
			// 配额：超限则拒绝（4xx，发件方会重试/退信）
			var qu model.User
			if err := db.Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(addr))).First(&qu).Error; err == nil && quota.Exceeded(db, &qu) {
				reply("452 4.2.2 Mailbox full")
				continue
			}
			to = addr
			reply("250 OK")
		case strings.HasPrefix(up, "DATA"):
			reply("354 End with .")
			inData = true
		case strings.HasPrefix(up, "QUIT"):
			reply("221 Bye")
			return
		case strings.HasPrefix(up, "RSET"):
			from, to = "", ""
			data.Reset()
			reply("250 OK")
		default:
			reply("250 OK")
		}
	}
}

func extractAddr(s string) string {
	a, b := strings.Index(s, "<"), strings.Index(s, ">")
	if a >= 0 && b > a {
		return s[a+1 : b]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

func saveMail(db *gorm.DB, from, to, raw string) {
	subject, body, htmlBody, atts := message.ParseInbound(raw)
	to = strings.ToLower(strings.TrimSpace(to))

	// 别名 / 转发：命中则投递到全部目标（本地进收件箱，外部自动转发）
	if al := alias.Match(db, to); al != nil {
		deliver.Forward(db, from, alias.Targets(al.Targets), subject, body, atts)
		if al.Keep {
			var u model.User
			if err := db.Where("LOWER(email) = ?", to).First(&u).Error; err == nil {
				deliver.ToUser(db, &u, from, to, "", "", subject, body, htmlBody, atts)
			}
		}
		return
	}

	// 普通收件人：按收件人找本地用户，找不到则丢弃（防垃圾占库）
	var u model.User
	if err := db.Where("LOWER(email) = ?", to).First(&u).Error; err != nil {
		return
	}
	deliver.ToUser(db, &u, from, to, "", "", subject, body, htmlBody, atts)
}

// recipientExists 判断收件人是否为本地已有用户或别名（RCPT 阶段就拒绝未知收件人，避免静默丢信）。
func recipientExists(db *gorm.DB, addr string) bool {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return false
	}
	var n int64
	db.Model(&model.User{}).Where("LOWER(email) = ?", addr).Count(&n)
	if n > 0 {
		return true
	}
	return alias.Match(db, addr) != nil
}
