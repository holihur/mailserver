package smtp

// 极简入站 SMTP（:2525），低内存：逐连接 goroutine，逐行解析，只收 DATA 存库。
// 生产收信请用 Postfix/Exim，此仅供本地联调。

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/alias"
	"mailserver/internal/deliver"
	"mailserver/internal/emailauth"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/quota"

	"gorm.io/gorm"
)

// 运行时配置钩子（main 注入，避免 smtp → runtimecfg → ... 的包循环）。
var (
	CatchAllFn  func() string
	AutoReplyFn func() (enabled bool, subject, body string)
)

func Serve(addr string, db *gorm.DB, maxBytes int64, dmarc string, tlsConf *tls.Config) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("smtp listen fail:", err)
		return
	}
	fmt.Println("smtp inbound on", addr, "max", maxBytes>>20, "MB", "tls=", tlsConf != nil)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, db, maxBytes, dmarc, tlsConf)
	}
}

func handle(c net.Conn, db *gorm.DB, maxBytes int64, dmarc string, tlsConf *tls.Config) {
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	remoteIP, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	encrypted := false
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	reply := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
	reply("220 mailserver ESMTP ready")

	var from string
	var rcpts []string
	var data strings.Builder
	inData, overLimit := false, false

	for {
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Minute))
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if overLimit {
				// 超限：丢弃剩余内容直至结束符，再回 552，保持协议同步
				if line == "." {
					inData, overLimit = false, false
					data.Reset()
					reply("552 5.3.4 message too large")
				}
				continue
			}
			if line == "." {
				inData = false
				for _, rcpt := range rcpts {
					saveMail(db, from, rcpt, remoteIP, data.String(), dmarc)
				}
				data.Reset()
				rcpts = nil
				reply("250 OK: queued")
				continue
			}
			if strings.HasPrefix(line, ".") {
				line = line[1:] // 透明传输：去掉一个前导点
			}
			data.WriteString(line + "\n")
			if int64(data.Len()) > maxBytes { // 超限：转入丢弃模式，防爆内存
				overLimit = true
				data.Reset()
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
			if tlsConf != nil && !encrypted {
				reply("250-Hello\r\n250-STARTTLS\r\n250 8BITMIME")
			} else {
				reply("250-Hello\r\n250 8BITMIME")
			}
		case strings.HasPrefix(up, "STARTTLS"):
			if tlsConf == nil || encrypted {
				reply("502 not supported")
				continue
			}
			reply("220 ready")
			tc := tls.Server(c, tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			c = tc
			r = bufio.NewReader(c)
			w = bufio.NewWriter(c)
			encrypted = true
			from, rcpts, data = "", nil, strings.Builder{}
			inData, overLimit = false, false
		case strings.HasPrefix(up, "MAIL FROM:"):
			from = extractAddr(line)
			rcpts = nil // 新邮件：清空上一封的收件人
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
			rcpts = append(rcpts, addr)
			reply("250 OK")
		case strings.HasPrefix(up, "DATA"):
			if len(rcpts) == 0 {
				reply("503 5.5.1 need RCPT first")
				continue
			}
			reply("354 End with .")
			inData = true
		case strings.HasPrefix(up, "QUIT"):
			reply("221 Bye")
			return
		case strings.HasPrefix(up, "RSET"):
			from = ""
			rcpts = nil
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

func saveMail(db *gorm.DB, from, to, remoteIP, raw, dmarcEnforce string) {
	in := message.ParseInboundFull(raw)
	subject, body, htmlBody, atts := in.Subject, in.Body, in.HTML, in.Attachments
	atts = message.Blobify(atts) // 附件内容寻址落盘（无 store 时保持 base64）
	to = strings.ToLower(strings.TrimSpace(to))
	// 环路保护：Received 跳数过高直接拒收（防两站互指滚雪球）
	if message.ReceivedCount(raw) > 50 {
		fmt.Printf("inbound loop rejected: received hops>50 from=%s to=%s\n", from, to)
		return
	}

	// 发件人认证（SPF/DKIM/DMARC）；SPF/DKIM 结果写入邮件，DMARC 按站点配置隔离/拒收（默认 none）
	res := emailauth.Evaluate(emailauth.NetLookup, net.ParseIP(remoteIP), from, []byte(raw))
	opts := deliver.Options{ReceiptTo: in.ReceiptTo, IsMDN: in.IsMDN, AuthResults: res.String(), Bulk: message.IsBulk(raw)}
	if res.DMARC == "fail" {
		switch dmarcEnforce {
		case "reject":
			fmt.Printf("inbound dmarc reject from=%s ip=%s\n", from, remoteIP)
			return
		case "quarantine":
			opts.Quarantine = true
		}
	}

	// 别名 / 转发：递归展开（带 visited 防自指/环路），本地进收件箱，外部自动转发
	if al := alias.Match(db, to); al != nil {
		visited := map[string]bool{to: true}
		var targets []string
		for _, t := range alias.Targets(al.Targets) {
			targets = append(targets, expandAliases(db, t, visited, 0)...)
		}
		if len(targets) == 0 {
			fmt.Printf("alias loop rejected: %s\n", to)
			return
		}
		deliver.Forward(db, from, targets, subject, body, atts)
		if al.Keep {
			var u model.User
			if err := db.Where("LOWER(email) = ?", to).First(&u).Error; err == nil {
				if in.IsMDN {
					markReceipt(db, u.ID, in.OriginalID)
				}
				deliver.ToUser(db, &u, from, to, "", "", subject, body, htmlBody, atts, opts)
			}
		}
		return
	}

	// 普通收件人：echo 虚拟地址、普通用户、未知收件人（catch-all）
	local, domain := splitAddr(to)
	if local == "echo" && hostedDomain(db, domain) {
		handleEcho(db, from, to, subject, body, htmlBody, atts, opts)
		return
	}
	var u model.User
	if err := db.Where("LOWER(email) = ?", to).First(&u).Error; err != nil {
		if ca := catchAllAddr(); ca != "" && hostedDomain(db, domain) {
			handleCatchAll(db, ca, from, to, subject, body, htmlBody, atts, opts)
			return
		}
		return
	}
	if in.IsMDN {
		markReceipt(db, u.ID, in.OriginalID)
	}
	deliver.ToUser(db, &u, from, to, "", "", subject, body, htmlBody, atts, opts)
	maybeAutoReply(db, u.Email, from, subject, body, opts.Bulk)
}

// expandAliases 递归展开别名到最终地址（visited + 深度上限，防别名自指/环路）。
func expandAliases(db *gorm.DB, addr string, visited map[string]bool, depth int) []string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" || depth > 10 || visited[addr] {
		return nil
	}
	visited[addr] = true
	al := alias.Match(db, addr)
	if al == nil {
		return []string{addr}
	}
	var out []string
	for _, t := range alias.Targets(al.Targets) {
		out = append(out, expandAliases(db, t, visited, depth+1)...)
	}
	return out
}

// markReceipt 把用户「已发送」中的原邮件标记为已收到对方回执。
func markReceipt(db *gorm.DB, uid uint, originalID string) {
	id := numericPrefix(originalID)
	if id == 0 {
		return
	}
	now := time.Now()
	db.Model(&model.Mail{}).Where("id = ? AND user_id = ? AND folder = ?", id, uid, "sent").
		Updates(map[string]any{"receipt_read": true, "receipt_at": now})
}

// numericPrefix 取 "<id>@host" 中的数字 id。
func numericPrefix(s string) uint {
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// recipientExists 判断收件人是否为本地已有用户或别名（RCPT 阶段就拒绝未知收件人，避免静默丢信）。
// echo 虚拟地址与配置了 catch-all 的托管域名也会接受。
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
	if alias.Match(db, addr) != nil {
		return true
	}
	local, domain := splitAddr(addr)
	if !hostedDomain(db, domain) {
		return false
	}
	if local == "echo" {
		return true
	}
	return catchAllAddr() != ""
}

func splitAddr(addr string) (local, domain string) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if i := strings.LastIndexByte(addr, '@'); i > 0 {
		return addr[:i], strings.Trim(addr[i+1:], ".")
	}
	return addr, ""
}

func hostedDomain(db *gorm.DB, domain string) bool {
	if domain == "" {
		return false
	}
	var n int64
	db.Model(&model.Domain{}).Where("LOWER(name) = ?", domain).Count(&n)
	return n > 0
}

func isEchoAddr(addr string) bool {
	local, _ := splitAddr(addr)
	return local == "echo"
}

func catchAllAddr() string {
	if CatchAllFn == nil {
		return ""
	}
	ca := strings.ToLower(strings.TrimSpace(CatchAllFn()))
	if ca == "" || !strings.Contains(ca, "@") {
		return ""
	}
	return ca
}

// handleEcho：任何人给 echo@ 发信，自动把原文回显给发件人（自己的 echo 与批量信不回，防环）。
func handleEcho(db *gorm.DB, from, to, subject, body, htmlBody, atts string, opts deliver.Options) {
	if opts.Bulk || from == "" || isEchoAddr(from) {
		return
	}
	replySubject := "Re: " + subject
	if strings.TrimSpace(subject) == "" {
		replySubject = "Re: (无主题)"
	}
	var sb strings.Builder
	sb.WriteString("这是 echo 自动回显。\n\n--- 原始邮件 ---\n")
	sb.WriteString("From: " + from + "\nTo: " + to + "\nSubject: " + subject + "\n\n")
	sb.WriteString(body)
	deliver.Forward(db, to, []string{from}, replySubject, sb.String(), "")
}

// handleCatchAll：未知收件人的邮件转投到指定邮箱（本地用户直接入库，外部地址入发件队列）。
func handleCatchAll(db *gorm.DB, catchAll, from, to, subject, body, htmlBody, atts string, opts deliver.Options) {
	var u model.User
	if err := db.Where("LOWER(email) = ?", catchAll).First(&u).Error; err == nil {
		deliver.ToUser(db, &u, from, to, "", "", subject, body, htmlBody, atts, opts)
	} else {
		deliver.Forward(db, from, []string{catchAll}, subject, body, atts)
	}
	maybeAutoReply(db, catchAll, from, subject, body, opts.Bulk)
}

// maybeAutoReply：全局自动回复（后台开关），同一发件人 24h 内只回一次。
func maybeAutoReply(db *gorm.DB, replyFrom, from, subject, body string, bulk bool) {
	if bulk || from == "" || replyFrom == "" || AutoReplyFn == nil {
		return
	}
	enabled, subj, tpl := AutoReplyFn()
	if !enabled || strings.EqualFold(from, replyFrom) || isEchoAddr(from) {
		return
	}
	var n int64
	db.Model(&model.Mail{}).
		Where("folder = ? AND LOWER(\"to\") = ? AND LOWER(\"from\") = ? AND created_at > ?",
			"sent", strings.ToLower(from), strings.ToLower(replyFrom), time.Now().Add(-24*time.Hour)).
		Count(&n)
	if n > 0 {
		return
	}
	if subj == "" {
		subj = "自动回复"
	}
	if strings.TrimSpace(tpl) == "" {
		tpl = "我已收到您的邮件，会尽快回复。"
	}
	deliver.Forward(db, replyFrom, []string{from}, subj, tpl+"\n\n--- 原邮件 ---\n"+body, "")
}
