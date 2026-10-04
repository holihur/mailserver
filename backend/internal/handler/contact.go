package handler

// 联系人（通讯录）：/api/contacts（GET 列表 / POST 新增）、/api/contacts/{id}（PATCH 改备注等 / DELETE）。
// /api/directory 返回站内启用用户，供写信时收件人下拉选择。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

type ContactBox struct{ DB *gorm.DB }

type contactInput struct {
	Name  string  `json:"name"`
	Email string  `json:"email"`
	Note  *string `json:"note"`
}

func (in contactInput) note() string {
	if in.Note == nil {
		return ""
	}
	return strings.TrimSpace(*in.Note)
}

// GET /api/contacts   POST /api/contacts {name,email,note}
func (h *ContactBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var cs []model.Contact
		h.DB.Where("user_id = ?", uid).Order("name, email").Find(&cs)
		if cs == nil {
			cs = []model.Contact{}
		}
		writeJSON(w, 200, cs)
	case "POST":
		var in contactInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(in.Email))
		if email == "" || !strings.Contains(email, "@") {
			writeJSON(w, 400, map[string]string{"error": "邮箱格式不正确"})
			return
		}
		c := model.Contact{UserID: uid, Email: email,
			Name: strings.TrimSpace(in.Name), Note: in.note()}
		// 同邮箱已存在则更新（upsert 语义，便于从收件人一键加入）
		var ex model.Contact
		if err := h.DB.Where("user_id = ? AND email = ?", uid, email).First(&ex).Error; err == nil {
			h.DB.Model(&ex).Updates(map[string]any{"name": c.Name, "note": c.Note})
			h.DB.First(&ex, ex.ID)
			writeJSON(w, 200, ex)
			return
		}
		if err := h.DB.Create(&c).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, c)
	default:
		w.WriteHeader(405)
	}
}

// PATCH /api/contacts/{id}   DELETE /api/contacts/{id}
func (h *ContactBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/contacts/")
	id = strings.Split(id, "/")[0]
	var c model.Contact
	if err := h.DB.Where("id = ? AND user_id = ?", id, uid).First(&c).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&c)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in contactInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd := map[string]any{}
		if strings.TrimSpace(in.Name) != "" {
			upd["name"] = strings.TrimSpace(in.Name)
		}
		if e := strings.ToLower(strings.TrimSpace(in.Email)); e != "" {
			if !strings.Contains(e, "@") {
				writeJSON(w, 400, map[string]string{"error": "邮箱格式不正确"})
				return
			}
			upd["email"] = e
		}
		if in.Note != nil { // 显式传 note 才更新，允许清空
			upd["note"] = in.note()
		}
		if len(upd) > 0 {
			if err := h.DB.Model(&c).Updates(upd).Error; err != nil {
				writeJSON(w, 500, map[string]string{"error": "保存失败"})
				return
			}
		}
		h.DB.Where("id = ? AND user_id = ?", id, uid).First(&c)
		writeJSON(w, 200, c)
	default:
		w.WriteHeader(405)
	}
}

// GET /api/directory -> 站内启用用户（收件人下拉用），返回 [{email,name}]
func (h *ContactBox) Directory(w http.ResponseWriter, r *http.Request) {
	if _, ok := uidOf(h.DB, w, r); !ok {
		return
	}
	var us []model.User
	h.DB.Where("disabled = ?", false).Order("email").Find(&us)
	out := make([]map[string]string, 0, len(us))
	for _, u := range us {
		out = append(out, map[string]string{"email": u.Email, "name": u.Name})
	}
	writeJSON(w, 200, out)
}
