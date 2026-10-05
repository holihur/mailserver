package handler

// 导入历史邮件（#51）：接受 mbox 或单封 EML（message/rfc822）原始内容，
// 解析后写入当前用户的指定文件夹。用于从其他邮箱迁移。

import (
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"mailserver/internal/htmlsanitize"
	"mailserver/internal/message"
	"mailserver/internal/model"
)

// mbox 分隔行：以 "From " 开头且含年份的 From_ 行。
var mboxSep = regexp.MustCompile(`(?m)^From .*\d{4}.*\r?\n`)

// POST /api/mails/import?folder=inbox   body: mbox 或单封 EML（上限 100MB）
func (m *MailBox) Import(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	folder := strings.TrimSpace(r.URL.Query().Get("folder"))
	if folder == "" {
		folder = "inbox"
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 100<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "读取失败或文件过大（上限 100MB）"})
		return
	}
	imported, skipped := 0, 0
	for _, raw := range splitMessages(string(data)) {
		from, to := fromTo(raw)
		if from == "" && to == "" {
			skipped++
			continue
		}
		subject, body, htmlBody, atts := message.ParseInbound(raw)
		atts = message.Blobify(atts)
		rec := model.Mail{UserID: uid, From: from, To: to, Subject: subject, Body: body,
			BodyHTML: htmlsanitize.Sanitize(htmlBody), Attachments: atts, Folder: folder, Read: true}
		if err := m.DB.Create(&rec).Error; err != nil {
			skipped++
			continue
		}
		imported++
	}
	writeJSON(w, 200, map[string]any{"ok": true, "imported": imported, "skipped": skipped})
}

// splitMessages 把 mbox 拆成多封；非 mbox（单封 EML）原样返回。
func splitMessages(s string) []string {
	if !strings.HasPrefix(s, "From ") {
		return []string{s}
	}
	idx := mboxSep.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return []string{s}
	}
	out := make([]string, 0, len(idx))
	for i := range idx {
		start := idx[i][1]
		end := len(s)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		if strings.TrimSpace(s[start:end]) != "" {
			out = append(out, s[start:end])
		}
	}
	return out
}

func fromTo(raw string) (string, string) {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(msg.Header.Get("From")), strings.TrimSpace(msg.Header.Get("To"))
}
