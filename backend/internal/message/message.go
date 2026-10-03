// Package message 组装 RFC5322 邮件（支持 Cc / Bcc / 附件），供发件队列、IMAP、POP3 共用。
// 附件以 JSON 存在 Mail.Attachments 里，Data 为 base64。
package message

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mailserver/internal/model"
)

type Attachment struct {
	Name string `json:"name"`
	Type string `json:"type"` // Content-Type，如 application/pdf
	Data string `json:"data"` // base64
	Size int    `json:"size"`
}

func ParseAttachments(s string) []Attachment {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var a []Attachment
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		return nil
	}
	return a
}

// Recipients 返回 To + Cc + Bcc 的全部地址（Bcc 只用于投递，不写进邮件头）。
func Recipients(m *model.Mail) []string {
	var out []string
	seen := map[string]bool{}
	for _, field := range []string{m.To, m.Cc, m.Bcc} {
		for _, a := range strings.Split(field, ",") {
			a = strings.TrimSpace(a)
			if a == "" || seen[strings.ToLower(a)] {
				continue
			}
			seen[strings.ToLower(a)] = true
			out = append(out, a)
		}
	}
	return out
}

func subject(m *model.Mail) string {
	if m.Subject == "" {
		return "(无主题)"
	}
	return m.Subject
}

// Parts 返回邮件头（有序）与正文（不含头部）。有附件时为 multipart/mixed。
func Parts(host string, m *model.Mail) (hdrs [][2]string, body string) {
	if host == "" {
		host = "mailserver.local"
	}
	hdrs = [][2]string{{"From", m.From}, {"To", m.To}}
	if strings.TrimSpace(m.Cc) != "" {
		hdrs = append(hdrs, [2]string{"Cc", m.Cc})
	}
	hdrs = append(hdrs,
		[2]string{"Subject", subject(m)},
		[2]string{"Date", m.CreatedAt.Format(time.RFC1123Z)},
		[2]string{"Message-ID", fmt.Sprintf("<%d@%s>", m.ID, host)},
		[2]string{"MIME-Version", "1.0"},
	)

	atts := ParseAttachments(m.Attachments)
	if len(atts) == 0 {
		hdrs = append(hdrs, [2]string{"Content-Type", "text/plain; charset=utf-8"})
		return hdrs, crlf(m.Body)
	}

	boundary := fmt.Sprintf("=_mailserver_%d_%d", m.ID, time.Now().UnixNano())
	hdrs = append(hdrs, [2]string{"Content-Type", `multipart/mixed; boundary="` + boundary + `"`})

	var b strings.Builder
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(crlf(m.Body))
	b.WriteString("\r\n")
	for _, a := range atts {
		ct := a.Type
		if ct == "" {
			ct = "application/octet-stream"
		}
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ct + `; name="` + a.Name + "\"\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"" + a.Name + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(wrap76(a.Data))
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return hdrs, b.String()
}

// Serialize 拼接额外头（如 DKIM-Signature）+ 邮件头 + 正文。
func Serialize(hdrs [][2]string, body string, extra ...string) []byte {
	var b strings.Builder
	for _, e := range extra {
		b.WriteString(e + "\r\n")
	}
	for _, h := range hdrs {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// Build 直接产出完整邮件（无 DKIM）。
func Build(host string, m *model.Mail) []byte {
	h, body := Parts(host, m)
	return Serialize(h, body)
}

func crlf(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76])
		b.WriteString("\r\n")
		s = s[76:]
	}
	b.WriteString(s)
	return b.String()
}
