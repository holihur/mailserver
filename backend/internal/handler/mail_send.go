package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/contacts"
	"mailserver/internal/external"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/push"
	"mailserver/internal/schedule"
)

func (m *MailBox) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	var in struct {
		From        string               `json:"from"`
		To          string               `json:"to"`
		Cc          string               `json:"cc"`
		Bcc         string               `json:"bcc"`
		Subject     string               `json:"subject"`
		Body        string               `json:"body"`
		Folder      string               `json:"folder"`
		SendAt      string               `json:"send_at"` // RFC3339，未来时间=定时发送
		Repeat      string               `json:"repeat"`  // "" | daily | weekly | monthly
		Receipt     bool                 `json:"receipt"` // 请求已读回执
		Attachments []message.Attachment `json:"attachments"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	var me model.User
	m.DB.First(&me, uid)
	// 发件身份：本人地址，或已绑定的第三方账号
	from := me.Email
	if f := strings.ToLower(strings.TrimSpace(in.From)); f != "" && f != strings.ToLower(me.Email) {
		if external.Find(m.DB, uid, f) == nil {
			writeJSON(w, 400, map[string]string{"error": "发件身份未授权"})
			return
		}
		from = f
	}
	folder := in.Folder
	if folder != "draft" {
		folder = "sent"
	}
	attJSON := ""
	if len(in.Attachments) > 0 {
		total := 0
		for _, a := range in.Attachments {
			total += len(a.Data) * 3 / 4
		}
		if total > 8<<20 {
			writeJSON(w, 413, map[string]string{"error": "附件过大（上限 8MB）"})
			return
		}
		b, _ := json.Marshal(in.Attachments)
		attJSON = string(b)
	}
	// 定时 / 周期性发送：未来时间的非草稿邮件先入库，由队列到点发出
	receiptTo := ""
	if in.Receipt {
		receiptTo = from
	}
	if folder == "sent" && strings.TrimSpace(in.SendAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(in.SendAt))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "无效的定时时间"})
			return
		}
		rep := strings.ToLower(strings.TrimSpace(in.Repeat))
		if !schedule.ValidRepeat(rep) {
			writeJSON(w, 400, map[string]string{"error": "无效的重复周期"})
			return
		}
		if t.After(time.Now()) {
			sm := model.ScheduledMail{UserID: uid, From: from, To: in.To, Cc: in.Cc, Bcc: in.Bcc,
				Subject: in.Subject, Body: in.Body, Attachments: attJSON, ReceiptTo: receiptTo, SendAt: t, Repeat: rep, Enabled: true}
			if err := m.DB.Create(&sm).Error; err != nil {
				writeJSON(w, 500, map[string]string{"error": "保存失败"})
				return
			}
			contacts.Collect(m.DB, uid, from, in.To, in.Cc, in.Bcc)
			writeJSON(w, 201, map[string]any{"id": sm.ID, "scheduled": true, "send_at": sm.SendAt, "repeat": sm.Repeat})
			return
		}
	}
	mail := model.Mail{UserID: uid, From: from, To: in.To, Cc: in.Cc, Bcc: in.Bcc,
		Subject: in.Subject, Body: in.Body, Attachments: attJSON, Folder: folder, Read: true, ReceiptTo: receiptTo}
	if folder == "sent" {
		mail.Status = "queued"
	}
	m.DB.Create(&mail)
	push.Notify(uid)
	if folder == "sent" {
		// 点发送即收录收件人到通讯录（投递时仍会兜底一次，幂等）
		contacts.Collect(m.DB, uid, from, in.To, in.Cc, in.Bcc)
	}
	if folder == "sent" && m.MQ != nil {
		_ = m.MQ.EnqueueSend(mail.ID)
	}
	writeJSON(w, 201, mail)
}
