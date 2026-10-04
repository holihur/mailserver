package handler

// 应用专用密码（PAT）管理：用户为邮件客户端生成/查看/吊销令牌。
// 路由：/api/tokens（GET 列表 / POST 创建）、/api/tokens/{id}（DELETE 吊销）。
// 令牌仅属于当前登录用户；明文只在创建响应中返回一次。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type TokenBox struct{ DB *gorm.DB }

// GET /api/tokens   POST /api/tokens {name}
func (h *TokenBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var ts []model.MailToken
		h.DB.Where("user_id = ?", uid).Order("id DESC").Find(&ts)
		if ts == nil {
			ts = []model.MailToken{}
		}
		writeJSON(w, 200, ts)
	case "POST":
		var in struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "邮件客户端"
		}
		if len([]rune(name)) > 60 {
			name = string([]rune(name)[:60])
		}
		plain, prefix, err := auth.NewMailToken()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "生成失败"})
			return
		}
		t := model.MailToken{UserID: uid, Name: name, Prefix: prefix, Hash: auth.HashMailToken(plain)}
		if err := h.DB.Create(&t).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		// token 为一次性明文，前端需提示用户立即复制。
		writeJSON(w, 201, map[string]any{
			"id": t.ID, "name": t.Name, "prefix": t.Prefix,
			"token": plain, "created_at": t.CreatedAt,
		})
	default:
		w.WriteHeader(405)
	}
}

// DELETE /api/tokens/{id}
func (h *TokenBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "DELETE" {
		w.WriteHeader(405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tokens/")
	id = strings.Split(id, "/")[0]
	res := h.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&model.MailToken{})
	if res.Error != nil {
		writeJSON(w, 500, map[string]string{"error": "删除失败"})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
