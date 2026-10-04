package handler

// 定时 / 周期性发送管理：/api/scheduled（GET 列表）、/api/scheduled/{id}（PATCH 启停或改期、DELETE 取消）。
// 创建走 POST /api/mails（带 send_at / repeat）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/schedule"

	"gorm.io/gorm"
)

type ScheduledBox struct{ DB *gorm.DB }

// GET /api/scheduled
func (h *ScheduledBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	var list []model.ScheduledMail
	h.DB.Where("user_id = ?", uid).Order("send_at").Find(&list)
	if list == nil {
		list = []model.ScheduledMail{}
	}
	writeJSON(w, 200, list)
}

// PATCH /api/scheduled/{id} {enabled,send_at,repeat}   DELETE /api/scheduled/{id}
func (h *ScheduledBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/scheduled/")
	id = strings.Split(id, "/")[0]
	var sm model.ScheduledMail
	if err := h.DB.Where("id = ? AND user_id = ?", id, uid).First(&sm).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&sm)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in struct {
			Enabled *bool   `json:"enabled"`
			SendAt  *string `json:"send_at"`
			Repeat  *string `json:"repeat"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd := map[string]any{}
		if in.Enabled != nil {
			upd["enabled"] = *in.Enabled
		}
		if in.Repeat != nil {
			rep := strings.ToLower(strings.TrimSpace(*in.Repeat))
			if !schedule.ValidRepeat(rep) {
				writeJSON(w, 400, map[string]string{"error": "无效的重复周期"})
				return
			}
			upd["repeat"] = rep
		}
		if in.SendAt != nil {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.SendAt))
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "无效的发送时间"})
				return
			}
			upd["send_at"] = t
		}
		if len(upd) > 0 {
			h.DB.Model(&sm).Updates(upd)
		}
		h.DB.First(&sm, sm.ID)
		writeJSON(w, 200, sm)
	default:
		w.WriteHeader(405)
	}
}
