package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type MailBox struct{ DB *gorm.DB }

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
	tx := m.DB.Where("user_id = ? AND folder = ?", uid, folder).Order("id DESC")
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("subject LIKE ? OR `from` LIKE ? OR `to` LIKE ?", like, like, like)
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
		m.DB.Model(&mail).Update("folder", "trash")
		writeJSON(w, 200, map[string]string{"ok": "true"})
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

// POST /api/mails  {to,subject,body,folder:sent|draft}
func (m *MailBox) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	var in struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Folder  string `json:"folder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	var me model.User
	m.DB.First(&me, uid)
	folder := in.Folder
	if folder != "draft" {
		folder = "sent"
	}
	mail := model.Mail{UserID: uid, From: me.Email, To: in.To, Subject: in.Subject, Body: in.Body, Folder: folder, Read: true}
	m.DB.Create(&mail)
	// TODO: 若配置了中继，在此用 net/smtp 投递；本地 demo 只落库
	writeJSON(w, 201, mail)
}
