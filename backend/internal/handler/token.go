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
			Name         string `json:"name"`
			AllowedCIDRs string `json:"allowed_cidrs"`
			Scopes       string `json:"scopes"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "邮件客户端"
		}
		if len([]rune(name)) > 60 {
			name = string([]rune(name)[:60])
		}
		cidrs := strings.TrimSpace(in.AllowedCIDRs)
		if err := auth.ValidCIDRs(cidrs); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		scopes := auth.NormalizeScopes(in.Scopes)
		if scopes == "" {
			scopes = "imap,smtp" // 新令牌默认最小权限；如需全权限请显式传全部 scopes
		}
		if err := auth.ValidScopes(scopes); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		plain, prefix, err := auth.NewMailToken()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "生成失败"})
			return
		}
		t := model.MailToken{UserID: uid, Name: name, Prefix: prefix, Hash: auth.HashMailToken(plain), AllowedCIDRs: cidrs, Scopes: scopes}
		if err := h.DB.Create(&t).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		// token 为一次性明文，前端需提示用户立即复制。
		writeJSON(w, 201, map[string]any{
			"id": t.ID, "name": t.Name, "prefix": t.Prefix,
			"allowed_cidrs": t.AllowedCIDRs, "scopes": t.Scopes,
			"token": plain, "created_at": t.CreatedAt,
		})
	default:
		w.WriteHeader(405)
	}
}

// DELETE /api/tokens/{id}   PATCH /api/tokens/{id} {name, allowed_cidrs}
func (h *TokenBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tokens/")
	id = strings.Split(id, "/")[0]
	var t model.MailToken
	if err := h.DB.Where("id = ? AND user_id = ?", id, uid).First(&t).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&t)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in struct {
			Name         *string `json:"name"`
			AllowedCIDRs *string `json:"allowed_cidrs"`
			Scopes       *string `json:"scopes"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd := map[string]any{}
		if in.Name != nil {
			upd["name"] = strings.TrimSpace(*in.Name)
		}
		if in.AllowedCIDRs != nil {
			cidrs := strings.TrimSpace(*in.AllowedCIDRs)
			if err := auth.ValidCIDRs(cidrs); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			upd["allowed_cidrs"] = cidrs
		}
		if in.Scopes != nil {
			scopes := auth.NormalizeScopes(*in.Scopes)
			if err := auth.ValidScopes(scopes); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			upd["scopes"] = scopes
		}
		if len(upd) > 0 {
			h.DB.Model(&t).Updates(upd)
		}
		h.DB.First(&t, t.ID)
		writeJSON(w, 200, t)
	default:
		w.WriteHeader(405)
	}
}
