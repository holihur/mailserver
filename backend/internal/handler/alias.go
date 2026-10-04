package handler

// 收件人别名 / 转发（管理员）：/api/admin/aliases（GET 列表 / POST 新建）、/api/admin/aliases/{id}（PATCH/DELETE）。
// 把发给某地址（或整域 catch-all）的邮件投递到一个或多个目标：本地用户进收件箱，外部地址自动转发。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/alias"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"

	"gorm.io/gorm"
)

type AliasBox struct {
	DB          *gorm.DB
	RT          *runtimecfg.Store
	AdminEmails string
}

type aliasInput struct {
	Source  string `json:"source"`
	Targets string `json:"targets"`
	Keep    *bool  `json:"keep"`
	Enabled *bool  `json:"enabled"`
}

func (in aliasInput) normalize(partial bool) (map[string]any, error) {
	upd := map[string]any{}
	src := strings.ToLower(strings.TrimSpace(in.Source))
	if src != "" {
		if !alias.ValidSource(src) {
			return nil, errText("来源需为 abc@example.com 或 @example.com")
		}
		upd["source"] = src
	} else if !partial {
		return nil, errText("来源不能为空")
	}
	if strings.TrimSpace(in.Targets) != "" {
		targets := alias.Targets(in.Targets)
		if len(targets) == 0 {
			return nil, errText("目标地址不能为空")
		}
		upd["targets"] = strings.Join(targets, ", ")
	} else if !partial {
		return nil, errText("目标地址不能为空")
	}
	if in.Keep != nil {
		upd["keep"] = *in.Keep
	} else if !partial {
		upd["keep"] = false
	}
	if in.Enabled != nil {
		upd["enabled"] = *in.Enabled
	} else if !partial {
		upd["enabled"] = true
	}
	return upd, nil
}

// GET/POST /api/admin/aliases
func (h *AliasBox) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		var as []model.MailAlias
		h.DB.Order("source").Find(&as)
		if as == nil {
			as = []model.MailAlias{}
		}
		writeJSON(w, 200, as)
	case "POST":
		var in aliasInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd, err := in.normalize(false)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		a := model.MailAlias{}
		a.Source, _ = upd["source"].(string)
		a.Targets, _ = upd["targets"].(string)
		a.Keep, _ = upd["keep"].(bool)
		a.Enabled, _ = upd["enabled"].(bool)
		if err := h.DB.Create(&a).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, a)
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/admin/aliases/{id}
func (h *AliasBox) One(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/aliases/")
	id = strings.Split(id, "/")[0]
	var a model.MailAlias
	if err := h.DB.Where("id = ?", id).First(&a).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&a)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in aliasInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd, err := in.normalize(true)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if len(upd) > 0 {
			h.DB.Model(&a).Updates(upd)
		}
		h.DB.Where("id = ?", id).First(&a)
		writeJSON(w, 200, a)
	default:
		w.WriteHeader(405)
	}
}
