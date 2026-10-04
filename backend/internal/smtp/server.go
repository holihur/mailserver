package smtp

// 极简入站 SMTP（:2525），低内存：逐连接 goroutine，逐行解析，只收 DATA 存库。
// 生产收信请用 Postfix/Exim，此仅供本地联调。

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/rules"

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
	// 按收件人找本地用户，找不到则丢弃（防垃圾占库）
	var u model.User
	if err := db.Where("email = ?", to).First(&u).Error; err != nil {
		return
	}
	subject, body, atts := message.ParseInbound(raw)
	// CEL 规则引擎：命中则投递到目标文件夹（默认 trash）
	folder := rules.Apply(db, u.ID, rules.Input{
		From: from, To: to, Subject: subject, Body: body,
		Size: len(raw), Attachments: len(message.ParseAttachments(atts)),
	}, "inbox").Folder
	db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Subject: subject, Body: body, Attachments: atts, Folder: folder})
}

// recipientExists 判断收件人是否为本地已有用户（RCPT 阶段就拒绝未知收件人，避免静默丢信）。
func recipientExists(db *gorm.DB, addr string) bool {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return false
	}
	var n int64
	db.Model(&model.User{}).Where("LOWER(email) = ?", addr).Count(&n)
	return n > 0
}
