package handler

// 第三方邮箱账号（用户自配）：/api/external（GET 列表 / POST 新建）、/api/external/{id}（PATCH/DELETE）、
// /api/external/{id}/test（测试 IMAP 拉取）。密码加密存储，接口不回传明文。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/external"
	"mailserver/internal/model"
	"mailserver/internal/secret"

	"gorm.io/gorm"
)

type ExternalBox struct {
	DB *gorm.DB
	MQ interface {
		EnqueueExtSync(accountID uint, history bool, limit int) error
	}
}

type externalView struct {
	model.ExternalAccount
	IMAPPassSet bool `json:"imap_pass_set"`
	SMTPPassSet bool `json:"smtp_pass_set"`
}

func viewOf(a model.ExternalAccount) externalView {
	return externalView{ExternalAccount: a, IMAPPassSet: a.IMAPPass != "", SMTPPassSet: a.SMTPPass != ""}
}

type externalInput struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	IMAPHost    string `json:"imap_host"`
	IMAPPort    string `json:"imap_port"`
	IMAPSSL     *bool  `json:"imap_ssl"`
	IMAPUser    string `json:"imap_user"`
	IMAPPass    string `json:"imap_pass"`
	SMTPHost    string `json:"smtp_host"`
	SMTPPort    string `json:"smtp_port"`
	SMTPSSL     *bool  `json:"smtp_ssl"`
	SMTPUser    string `json:"smtp_user"`
	SMTPPass    string `json:"smtp_pass"`
	Enabled     *bool  `json:"enabled"`
	SyncHistory *bool  `json:"sync_history"`
	SyncLimit   *int   `json:"sync_limit"`
}

func (in externalInput) apply(a *model.ExternalAccount, partial bool) error {
	if e := strings.ToLower(strings.TrimSpace(in.Email)); e != "" {
		if !strings.Contains(e, "@") {
			return errText("邮箱格式不正确")
		}
		a.Email = e
	} else if !partial {
		return errText("邮箱不能为空")
	}
	if h := strings.TrimSpace(in.IMAPHost); h != "" {
		a.IMAPHost = h
	} else if !partial {
		return errText("IMAP 服务器不能为空")
	}
	a.Name = strings.TrimSpace(in.Name)
	a.IMAPPort = strings.TrimSpace(in.IMAPPort)
	a.IMAPUser = strings.TrimSpace(in.IMAPUser)
	if a.IMAPUser == "" {
		a.IMAPUser = a.Email
	}
	a.SMTPHost = strings.TrimSpace(in.SMTPHost)
	a.SMTPPort = strings.TrimSpace(in.SMTPPort)
	a.SMTPUser = strings.TrimSpace(in.SMTPUser)
	if a.SMTPUser == "" {
		a.SMTPUser = a.Email
	}
	if in.IMAPSSL != nil {
		a.IMAPSSL = *in.IMAPSSL
	}
	if in.SMTPSSL != nil {
		a.SMTPSSL = *in.SMTPSSL
	}
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	} else if !partial {
		a.Enabled = true
	}
	if in.SyncHistory != nil {
		a.SyncHistory = *in.SyncHistory
	}
	if in.SyncLimit != nil {
		a.SyncLimit = *in.SyncLimit
	}
	if in.IMAPPass != "" {
		enc, err := secret.Encrypt([]byte(in.IMAPPass))
		if err != nil {
			return errText("密码加密失败")
		}
		a.IMAPPass = enc
	}
	if in.SMTPPass != "" {
		enc, err := secret.Encrypt([]byte(in.SMTPPass))
		if err != nil {
			return errText("密码加密失败")
		}
		a.SMTPPass = enc
	}
	return nil
}

// GET/POST /api/external
func (h *ExternalBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var as []model.ExternalAccount
		h.DB.Where("user_id = ?", uid).Order("id").Find(&as)
		out := make([]externalView, 0, len(as))
		for _, a := range as {
			out = append(out, viewOf(a))
		}
		writeJSON(w, 200, out)
	case "POST":
		var in externalInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		a := model.ExternalAccount{UserID: uid}
		if err := in.apply(&a, false); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := h.DB.Create(&a).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, viewOf(a))
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/external/{id}   POST /api/external/{id}/test
func (h *ExternalBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/external/")
	parts := strings.Split(rest, "/")
	var a model.ExternalAccount
	if err := h.DB.Where("id = ? AND user_id = ?", parts[0], uid).First(&a).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == "POST" {
		n, err := external.Sync(h.DB, &a)
		if err != nil {
			h.DB.Model(&a).Updates(map[string]any{"last_error": err.Error()})
			writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.DB.Model(&a).Update("last_error", "")
		writeJSON(w, 200, map[string]any{"ok": true, "imported": n})
		return
	}
	// POST /api/external/{id}/sync {history?:bool, limit?:int}
	if len(parts) == 2 && parts[1] == "sync" && r.Method == "POST" {
		var in struct {
			History bool `json:"history"`
			Limit   int  `json:"limit"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
		// 优先入 asynq 异步队列（不阻塞请求）；无 Redis 时同步执行
		if h.MQ != nil {
			if err := h.MQ.EnqueueExtSync(a.ID, in.History, in.Limit); err == nil {
				writeJSON(w, 202, map[string]any{"ok": true, "queued": true})
				return
			}
		}
		var n int
		var err error
		if in.History {
			n, err = external.SyncHistory(h.DB, &a, in.Limit)
		} else {
			n, err = external.Sync(h.DB, &a)
		}
		if err != nil {
			h.DB.Model(&a).Updates(map[string]any{"last_error": err.Error()})
			writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.DB.Model(&a).Updates(map[string]any{"last_error": "", "last_sync": time.Now()})
		writeJSON(w, 200, map[string]any{"ok": true, "imported": n})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&a)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in externalInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		if err := in.apply(&a, true); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := h.DB.Save(&a).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 200, viewOf(a))
	default:
		w.WriteHeader(405)
	}
}
