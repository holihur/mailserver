package handler

// Sieve 脚本管理：/api/sieve（GET 列表 / PUT 保存）、/api/sieve/{id}（DELETE）、
// /api/sieve/{id}/activate（设为启用）、/api/sieve/check（语法检查）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/sieve"

	"gorm.io/gorm"
)

type SieveBox struct{ DB *gorm.DB }

// GET /api/sieve   PUT /api/sieve {name, script}
func (h *SieveBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var ss []model.SieveScript
		h.DB.Where("user_id = ?", uid).Order("name").Find(&ss)
		if ss == nil {
			ss = []model.SieveScript{}
		}
		writeJSON(w, 200, ss)
	case "PUT", "POST":
		var in struct {
			Name   string `json:"name"`
			Script string `json:"script"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "main"
		}
		if len([]rune(name)) > 100 {
			writeJSON(w, 400, map[string]string{"error": "脚本名过长"})
			return
		}
		if _, err := sieve.Compile(in.Script); err != nil {
			writeJSON(w, 400, map[string]string{"error": "脚本语法错误: " + err.Error()})
			return
		}
		var sc model.SieveScript
		if err := h.DB.Where("user_id = ? AND name = ?", uid, name).First(&sc).Error; err == nil {
			h.DB.Model(&sc).Updates(map[string]any{"script": in.Script, "updated_at": time.Now()})
		} else {
			sc = model.SieveScript{UserID: uid, Name: name, Script: in.Script, UpdatedAt: time.Now()}
			if err := h.DB.Create(&sc).Error; err != nil {
				writeJSON(w, 500, map[string]string{"error": "保存失败"})
				return
			}
		}
		h.DB.Where("user_id = ? AND name = ?", uid, name).First(&sc)
		writeJSON(w, 200, sc)
	default:
		w.WriteHeader(405)
	}
}

// DELETE /api/sieve/{id}   POST /api/sieve/{id}/activate
func (h *SieveBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/sieve/")
	parts := strings.Split(rest, "/")
	var sc model.SieveScript
	if err := h.DB.Where("id = ? AND user_id = ?", parts[0], uid).First(&sc).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if len(parts) == 2 && parts[1] == "activate" && r.Method == "POST" {
		h.DB.Model(&model.SieveScript{}).Where("user_id = ?", uid).Update("active", false)
		h.DB.Model(&sc).Update("active", true)
		writeJSON(w, 200, map[string]any{"ok": true, "active": sc.Name})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&sc)
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		w.WriteHeader(405)
	}
}

// POST /api/sieve/check {script}
func (h *SieveBox) Check(w http.ResponseWriter, r *http.Request) {
	if _, ok := uidOf(h.DB, w, r); !ok {
		return
	}
	var in struct {
		Script string `json:"script"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	if _, err := sieve.Compile(in.Script); err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
