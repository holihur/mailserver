package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/external"
	"mailserver/internal/message"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type MailBox struct {
	DB *gorm.DB
	MQ interface{ EnqueueSend(mailID uint) error }
}

func uidOf(db *gorm.DB, w http.ResponseWriter, r *http.Request) (uint, bool) {
	uid, err := auth.UserID(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return 0, false
	}
	var u model.User
	if err := db.Select("id", "disabled").First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
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
		tx = tx.Where("subject LIKE ? OR \"from\" LIKE ? OR \"to\" LIKE ?", like, like, like)
	}
	var total int64
	tx.Model(&model.Mail{}).Count(&total)
	var items []model.Mail
	tx.Offset((page - 1) * size).Limit(size).Find(&items)
	if items == nil {
		items = []model.Mail{}
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items, "page": page})
}

// GET /api/mails/:id  PATCH /api/mails/:id  DELETE /api/mails/:id
func (m *MailBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/mails/")
	id = strings.Split(id, "/")[0]
	var mail model.Mail
	if err := m.DB.Where("id = ? AND user_id = ?", id, uid).First(&mail).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "GET":
		if !mail.Read {
			m.DB.Model(&mail).Update("read", true)
			mail.Read = true
		}
		writeJSON(w, 200, mail)
	case "PATCH":
		var in struct {
			Read    *bool   `json:"read"`
			Starred *bool   `json:"starred"`
			Folder  *string `json:"folder"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
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
		if len(upd) > 0 {
			m.DB.Model(&mail).Updates(upd)
		}
		m.DB.Where("id = ? AND user_id = ?", id, uid).First(&mail)
		writeJSON(w, 200, mail)
	case "DELETE":
		// 已在垃圾箱：彻底删除且不可恢复；否则移入垃圾箱（可恢复）。
		if mail.Folder == "trash" {
			m.DB.Delete(&mail)
			writeJSON(w, 200, map[string]any{"ok": true, "deleted": true})
			return
		}
		m.DB.Model(&mail).Update("folder", "trash")
		writeJSON(w, 200, map[string]any{"ok": true, "deleted": false})
	}
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
	// 清空垃圾箱：无需选中邮件，直接彻底删除当前用户全部 trash。
	if in.Action == "empty" {
		res := m.DB.Where("user_id = ? AND folder = ?", uid, "trash").Delete(&model.Mail{})
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
	mail := model.Mail{UserID: uid, From: from, To: in.To, Cc: in.Cc, Bcc: in.Bcc,
		Subject: in.Subject, Body: in.Body, Attachments: attJSON, Folder: folder, Read: true}
	if folder == "sent" {
		mail.Status = "queued"
	}
	m.DB.Create(&mail)
	if folder == "sent" && m.MQ != nil {
		_ = m.MQ.EnqueueSend(mail.ID)
	}
	writeJSON(w, 201, mail)
}
