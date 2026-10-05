package handler

// 管理员操作审计：对 /api/admin/ 的写操作统一记录（脱敏）。

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

func isWriteMethod(m string) bool {
	switch m {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(c int) {
	s.code = c
	s.ResponseWriter.WriteHeader(c)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = 200
	}
	return s.ResponseWriter.Write(b)
}

// AuditAdmin 包裹整个 mux：仅对 /api/admin/ 的写操作、且响应成功时落一条审计。
func AuditAdmin(db *gorm.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if db == nil || !strings.HasPrefix(r.URL.Path, "/api/admin/") || !isWriteMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.code >= 400 {
			return
		}
		uid, _, _, _ := auth.Access(r)
		var u model.User
		db.Select("email").First(&u, uid)
		ip := ClientIP(r)
		db.Create(&model.AuditLog{
			ActorID: uid, ActorEmail: u.Email, Action: r.Method, Target: r.URL.Path,
			Detail: redactJSON(body), IP: ip,
		})
		log.Printf("[AUDIT] actor=%s method=%s target=%s ip=%s", u.Email, r.Method, r.URL.Path, ip)
	})
}

// redactJSON 只保留字段名，敏感字段值替换为 ***；非 JSON 不记录正文。
func redactJSON(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	for k := range m {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "pass") || strings.Contains(lk, "secret") ||
			strings.Contains(lk, "key") || strings.Contains(lk, "token") {
			m[k] = "***"
		}
	}
	out, _ := json.Marshal(m)
	if len(out) > 800 {
		out = out[:800]
	}
	return string(out)
}

// AuditLogs GET /api/admin/audit -> 最近 200 条审计日志。
func (a *Admin) AuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	var logs []model.AuditLog
	a.DB.Order("id DESC").Limit(200).Find(&logs)
	if logs == nil {
		logs = []model.AuditLog{}
	}
	writeJSON(w, 200, logs)
}

// StartAuditRetention 每天清理超过 180 天的审计日志。
func StartAuditRetention(db *gorm.DB) {
	go func() {
		for {
			db.Where("created_at < ?", time.Now().AddDate(0, 0, -180)).Delete(&model.AuditLog{})
			time.Sleep(24 * time.Hour)
		}
	}()
}

// StartAuthRetention 每天清理：过期的登录会话 + 超过 90 天的登录历史。
// 避免 sessions / login_events 无限增长。
func StartAuthRetention(db *gorm.DB) {
	go func() {
		for {
			PurgeAuth(db)
			time.Sleep(24 * time.Hour)
		}
	}()
}

// PurgeAuth 执行一次清理，返回删除的（会话数, 登录历史数）。
func PurgeAuth(db *gorm.DB) (int64, int64) {
	s := db.Where("expires_at < ?", time.Now()).Delete(&model.Session{})
	e := db.Where("created_at < ?", time.Now().AddDate(0, 0, -90)).Delete(&model.LoginEvent{})
	return s.RowsAffected, e.RowsAffected
}
