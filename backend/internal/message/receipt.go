package message

import (
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"mailserver/internal/model"
)

// BuildMDN 组装一封已读回执（RFC 3798）：multipart/report + message/disposition-notification。
// m.From=回执发送者（原收件人），m.To=回执接收者（原发件人），m.ReceiptFor=原邮件 ID。
func BuildMDN(host string, m *model.Mail) []byte {
	if host == "" {
		host = "mailserver.local"
	}
	boundary := fmt.Sprintf("=_mdn_%d_%d", m.ID, time.Now().UnixNano())
	origID := fmt.Sprintf("<%d@%s>", m.ReceiptFor, host)
	hdrs := [][2]string{
		{"From", m.From},
		{"To", m.To},
		{"Subject", subject(m)},
		{"Date", m.CreatedAt.Format(time.RFC1123Z)},
		{"Message-ID", fmt.Sprintf("<%d@%s>", m.ID, host)},
		{"MIME-Version", "1.0"},
		{"Content-Type", `multipart/report; report-type=disposition-notification; boundary="` + boundary + `"`},
	}
	var b strings.Builder
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(crlf(m.Body))
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: message/disposition-notification\r\n\r\n")
	b.WriteString("Reporting-UA: Sweetcorn\r\n")
	b.WriteString("Final-Recipient: rfc822; " + m.From + "\r\n")
	b.WriteString("Original-Message-ID: " + origID + "\r\n")
	b.WriteString("Disposition: manual-action/MDN-sent-manually; displayed\r\n")
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return Serialize(hdrs, b.String())
}

// Inbound 收到的原始邮件解析结果（含已读回执相关字段）。
type Inbound struct {
	Subject     string
	Body        string
	HTML        string
	Attachments string
	ReceiptTo   string // Disposition-Notification-To 等：要求回执的地址
	IsMDN       bool   // 本条是一封已读回执
	OriginalID  string // MDN 引用的原邮件 Message-ID（去尖括号）
}

// ParseInboundFull 在 ParseInbound 基础上补充回执相关字段。
func ParseInboundFull(raw string) Inbound {
	subject, body, htmlBody, atts := ParseInbound(raw)
	in := Inbound{Subject: subject, Body: body, HTML: htmlBody, Attachments: atts}
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return in
	}
	in.ReceiptTo = firstReceiptAddr(msg.Header)
	mt, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	in.IsMDN = mt == "message/disposition-notification" ||
		(mt == "multipart/report" && strings.EqualFold(params["report-type"], "disposition-notification"))
	if in.IsMDN {
		in.OriginalID = findOriginalID(raw, msg.Header)
	}
	return in
}

// ReceivedCount 统计原始邮件中 Received 头的数量（环路跳数）。
func ReceivedCount(raw string) int {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return 0
	}
	return len(msg.Header["Received"])
}

// HasDeliveredTo 判断已存在 Delivered-To: 指定地址（环路去重）。
func HasDeliveredTo(raw, addr string) bool {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return false
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	for _, v := range msg.Header["Delivered-To"] {
		if a, err := mail.ParseAddress(strings.TrimSpace(v)); err == nil && strings.ToLower(a.Address) == addr {
			return true
		}
	}
	return false
}

func firstReceiptAddr(h mail.Header) string {
	for _, k := range []string{"Disposition-Notification-To", "Return-Receipt-To", "X-Confirm-Reading-To"} {
		if v := strings.TrimSpace(h.Get(k)); v != "" {
			if a := firstEmail(v); a != "" {
				return a
			}
		}
	}
	return ""
}

func firstEmail(s string) string {
	if a, err := mail.ParseAddress(s); err == nil {
		return strings.ToLower(strings.TrimSpace(a.Address))
	}
	if i, j := strings.Index(s, "<"), strings.Index(s, ">"); i >= 0 && j > i {
		return strings.ToLower(strings.TrimSpace(s[i+1 : j]))
	}
	if f := strings.Fields(s); len(f) > 0 {
		return strings.ToLower(f[0])
	}
	return ""
}

// findOriginalID 依次从 Original-Message-ID / In-Reply-To / References 中取原邮件 ID。
// Original-Message-ID 位于 message/disposition-notification 部分，需按行在原文中查找。
func findOriginalID(raw string, h mail.Header) string {
	for _, k := range []string{"Original-Message-ID", "In-Reply-To", "References"} {
		if v := headerOrBodyValue(raw, h, k); v != "" {
			return stripAngle(v)
		}
	}
	return ""
}

func headerOrBodyValue(raw string, h mail.Header, key string) string {
	if v := strings.TrimSpace(h.Get(key)); v != "" {
		return v
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if len(line) > len(key) && strings.EqualFold(line[:len(key)], key) && line[len(key)] == ':' {
			return strings.TrimSpace(line[len(key)+1:])
		}
	}
	return ""
}

func stripAngle(s string) string {
	s = strings.TrimSpace(s)
	if i, j := strings.Index(s, "<"), strings.Index(s, ">"); i >= 0 && j > i {
		return strings.TrimSpace(s[i+1 : j])
	}
	if f := strings.Fields(s); len(f) > 0 {
		return strings.Trim(f[0], "<>")
	}
	return strings.Trim(s, "<>")
}
