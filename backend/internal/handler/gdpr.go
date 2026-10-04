package handler

// GDPR 支持：数据可携（导出）与删除权（账户注销）。
// GET  /api/gdpr/export  -> 下载当前用户的全部个人数据（JSON）
// POST /api/gdpr/delete  {password} -> 删除账户及其全部数据

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mailserver/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type GDPRBox struct{ DB *gorm.DB }

// GET /api/gdpr/export
func (h *GDPRBox) Export(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	var u model.User
	if err := h.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	var mails []model.Mail
	h.DB.Where("user_id = ?", uid).Order("id").Find(&mails)
	var contacts []model.Contact
	h.DB.Where("user_id = ?", uid).Find(&contacts)
	var rules []model.MailRule
	h.DB.Where("user_id = ?", uid).Find(&rules)
	var tokens []model.MailToken
	h.DB.Where("user_id = ?", uid).Find(&tokens)

	// 令牌只导出元数据（不含明文/摘要）
	type tokenMeta struct {
		Name      string     `json:"name"`
		Prefix    string     `json:"prefix"`
		CreatedAt time.Time  `json:"created_at"`
		LastUsed  *time.Time `json:"last_used"`
	}
	ts := make([]tokenMeta, 0, len(tokens))
	for _, x := range tokens {
		ts = append(ts, tokenMeta{Name: x.Name, Prefix: x.Prefix, CreatedAt: x.CreatedAt, LastUsed: x.LastUsed})
	}

	payload := map[string]any{
		"exported_at": time.Now().Format(time.RFC3339),
		"account": map[string]any{
			"email": u.Email, "name": u.Name, "created_at": u.CreatedAt, "totp_enabled": u.TOTPEnabled,
		},
		"mails":       mails,
		"contacts":    contacts,
		"rules":       rules,
		"mail_tokens": ts,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"sweetcorn-export-%d.json\"", uid))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

// POST /api/gdpr/delete {password}
func (h *GDPRBox) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	var u model.User
	if err := h.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(in.Password)) != nil {
		writeJSON(w, 403, map[string]string{"error": "密码错误"})
		return
	}
	// 防锁死：不允许删除最后一个管理员
	if u.Admin {
		var n int64
		h.DB.Model(&model.User{}).Where("admin = ? AND id <> ?", true, u.ID).Count(&n)
		if n == 0 {
			writeJSON(w, 400, map[string]string{"error": "不能删除最后一个管理员账户"})
			return
		}
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&model.Mail{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.Contact{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.MailRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.MailToken{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, uid).Error
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "删除失败"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
