package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/contacts"
	"mailserver/internal/external"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/schedule"

	"gorm.io/gorm"
)

type MailBox struct {
	DB *gorm.DB
	MQ interface{ EnqueueSend(mailID uint) error }
}

func uidOf(db *gorm.DB, w http.ResponseWriter, r *http.Request) (uint, bool) {
	uid, ver, err := auth.Access(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return 0, false
	}
	var u model.User
	if err := db.Select("id", "disabled", "token_version").First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return 0, false
	}
	if u.TokenVersion != ver {
		writeJSON(w, 401, map[string]string{"error": "登录已失效，请重新登录"})
		return 0, false
	}
	if u.Disabled {
		writeJSON(w, 403, map[string]string{"error": "账号已禁用"})
		return 0, false
	}
	return uid, true
}

// GET /api/mails?folder=inbox&q=&page=1&pageSize=20
func (m *MailBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "inbox"
	}
	q := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size := 20
	// 排序（白名单，防注入）
	order := "id DESC"
	switch r.URL.Query().Get("sort") {
	case "oldest":
		order = "id ASC"
	case "subject":
		order = "subject ASC, id DESC"
	case "sender":
		order = "\"from\" ASC, id DESC"
	}
	tx := m.DB.Where("user_id = ? AND folder = ?", uid, folder).Order(order)
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("subject ILIKE ? OR \"from\" ILIKE ? OR \"to\" ILIKE ? OR body ILIKE ?", like, like, like, like)
	}
	var total int64
	tx.Model(&model.Mail{}).Count(&total)
	var items []model.Mail
	tx.Offset((page - 1) * size).Limit(size).Find(&items)
	if items == nil {
		items = []model.Mail{}
	}
	for i := range items {
		items[i].Attachments = message.HydrateAttachments(items[i].Attachments)
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items, "page": page})
}

// GET /api/mails/:id  PATCH /api/mails/:id  DELETE /api/mails/:id
func (m *MailBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/mails/")
	id := strings.Split(rest, "/")[0]
	var mail model.Mail
	if err := m.DB.Where("id = ? AND user_id = ?", id, uid).First(&mail).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	// POST /api/mails/{id}/undo：撤销发送（仅限尚未真正发出的邮件）
	if strings.HasSuffix(rest, "/undo") {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if mail.Relayed || mail.Status == "sent" || mail.Status == "sending" {
			writeJSON(w, 409, map[string]string{"error": "已发出，无法撤销"})
			return
		}
		m.DB.Delete(&mail) // 定时任务触发时找不到邮件会自动跳过
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	// POST /api/mails/{id}/receipt：回复已读回执（MDN）
	if strings.HasSuffix(rest, "/receipt") {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if mail.Folder != "inbox" || strings.TrimSpace(mail.ReceiptTo) == "" || mail.ReceiptSent {
			writeJSON(w, 400, map[string]string{"error": "该邮件无需回执"})
			return
		}
		var me model.User
		m.DB.First(&me, uid)
		mdn := model.Mail{UserID: uid, From: me.Email, To: mail.ReceiptTo,
			Subject: "已读回执: " + mail.Subject, Body: mdnBody(me.Email, mail),
			Folder: "sent", Read: true, Status: "queued", IsMDN: true, ReceiptFor: mail.ID}
		if err := m.DB.Create(&mdn).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		if m.MQ != nil {
			_ = m.MQ.EnqueueSend(mdn.ID)
		}
		m.DB.Model(&mail).Update("receipt_sent", true)
		writeJSON(w, 200, map[string]any{"ok": true, "id": mdn.ID})
		return
	}
	switch r.Method {
	case "GET":
		if !mail.Read {
			m.DB.Model(&mail).Update("read", true)
			mail.Read = true
		}
		mail.Attachments = message.HydrateAttachments(mail.Attachments)
		writeJSON(w, 200, mail)
	case "PATCH":
		var in struct {
			Read    *bool   `json:"read"`
			Starred *bool   `json:"starred"`
			Folder  *string `json:"folder"`
			To      *string `json:"to"`
			Cc      *string `json:"cc"`
			Bcc     *string `json:"bcc"`
			Subject *string `json:"subject"`
			Body    *string `json:"body"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&in)
		upd := map[string]any{}
		if in.Read != nil {
			upd["read"] = *in.Read
		}
		if in.Starred != nil {
			upd["starred"] = *in.Starred
		}
		if in.Folder != nil {
			upd["folder"] = *in.Folder
		}
		if in.To != nil {
			upd["to"] = *in.To
		}
		if in.Cc != nil {
			upd["cc"] = *in.Cc
		}
		if in.Bcc != nil {
			upd["bcc"] = *in.Bcc
		}
		if in.Subject != nil {
			upd["subject"] = *in.Subject
		}
		if in.Body != nil {
			upd["body"] = *in.Body
		}
		if len(upd) > 0 {
			m.DB.Model(&mail).Updates(upd)
		}
		m.DB.Where("id = ? AND user_id = ?", id, uid).First(&mail)
		writeJSON(w, 200, mail)
	case "DELETE":
		// 已在「已删除」：彻底删除；在垃圾箱：移入「已删除」；否则：移入垃圾箱。
		if mail.Folder == "deleted" {
			m.DB.Delete(&mail)
			writeJSON(w, 200, map[string]any{"ok": true, "deleted": true})
			return
		}
		if mail.Folder == "trash" {
			m.DB.Model(&mail).Update("folder", "deleted")
			writeJSON(w, 200, map[string]any{"ok": true, "deleted": false})
			return
		}
		m.DB.Model(&mail).Update("folder", "trash")
		writeJSON(w, 200, map[string]any{"ok": true, "deleted": false})
	}
}

// mdnBody 生成已读回执的纯文本说明部分。
func mdnBody(who string, mail model.Mail) string {
	return fmt.Sprintf("这是 %s 发出的已读回执。\n\n原邮件主题: %s\n原邮件发件人: %s\n阅读时间: %s\n",
		who, mail.Subject, mail.From, time.Now().Format(time.RFC1123))
}

// GET /api/outbox  最近 20 封发件的投递状态（25 被封排障用）
func (m *MailBox) Outbox(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	var items []model.Mail
	m.DB.Where("user_id = ? AND folder = ?", uid, "sent").Order("id DESC").Limit(20).Find(&items)
	if items == nil {
		items = []model.Mail{}
	}
	writeJSON(w, 200, items)
}

// GET /api/mails/unread -> {folder: 未读数}
func (m *MailBox) Unread(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	type row struct {
		Folder string
		N      int64
	}
	var rows []row
	m.DB.Model(&model.Mail{}).Select("folder, COUNT(*) AS n").
		Where("user_id = ?", uid).Where(map[string]any{"read": false}).
		Group("folder").Scan(&rows)
	out := map[string]int64{}
	for _, x := range rows {
		out[x.Folder] = x.N
	}
	writeJSON(w, 200, out)
}

// POST /api/mails/batch {ids:[...], action:trash|delete|star|unstar|read|unread|move|empty, folder?}
func (m *MailBox) Batch(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		IDs    []uint `json:"ids"`
		Action string `json:"action"`
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	// 清空垃圾箱：移入「已删除」（软删除，可再恢复或彻底删除）。
	if in.Action == "empty" {
		res := m.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", uid, "trash").Update("folder", "deleted")
		writeJSON(w, 200, map[string]any{"ok": true, "count": res.RowsAffected})
		return
	}
	if len(in.IDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "未选择邮件"})
		return
	}
	if len(in.IDs) > 500 {
		in.IDs = in.IDs[:500]
	}
	tx := m.DB.Where("user_id = ? AND id IN ?", uid, in.IDs)
	switch in.Action {
	case "delete":
		tx.Model(&model.Mail{}).Update("folder", "deleted")
	case "purge":
		tx.Delete(&model.Mail{})
	case "trash":
		tx.Model(&model.Mail{}).Update("folder", "trash")
	case "star":
		tx.Model(&model.Mail{}).Update("starred", true)
	case "unstar":
		tx.Model(&model.Mail{}).Update("starred", false)
	case "read":
		tx.Model(&model.Mail{}).Update("read", true)
	case "unread":
		tx.Model(&model.Mail{}).Update("read", false)
	case "move":
		if in.Folder == "" {
			writeJSON(w, 400, map[string]string{"error": "缺少目标文件夹"})
			return
		}
		tx.Model(&model.Mail{}).Update("folder", in.Folder)
	default:
		writeJSON(w, 400, map[string]string{"error": "不支持的操作"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(in.IDs)})
}

// POST /api/mails  {to,subject,body,folder:sent|draft}
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
	if folder == "sent" {
		// 点发送即收录收件人到通讯录（投递时仍会兜底一次，幂等）
		contacts.Collect(m.DB, uid, from, in.To, in.Cc, in.Bcc)
	}
	if folder == "sent" && m.MQ != nil {
		_ = m.MQ.EnqueueSend(mail.ID)
	}
	writeJSON(w, 201, mail)
}
