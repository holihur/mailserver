package handler

// CEL 收信规则：用户级 /api/rules，整站级 /api/admin/rules（仅管理员）。
// 命中后把邮件投递到指定文件夹（默认 trash）。表达式返回 bool。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/model"
	"mailserver/internal/rules"
	"mailserver/internal/runtimecfg"

	"gorm.io/gorm"
)

// RuleBox Site=true 时管理整站规则（owner 固定 0，需管理员）。
type RuleBox struct {
	DB          *gorm.DB
	RT          *runtimecfg.Store
	AdminEmails string
	Site        bool
}

func (h *RuleBox) owner(w http.ResponseWriter, r *http.Request) (uint, bool) {
	if h.Site {
		if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
			return 0, false
		}
		return 0, true
	}
	return uidOf(h.DB, w, r)
}

type ruleInput struct {
	Name       string `json:"name"`
	Enabled    *bool  `json:"enabled"`
	Priority   *int   `json:"priority"`
	Expression string `json:"expression"`
	Action     string `json:"action"`
	Folder     string `json:"folder"`
}

// normalize 校验并生成入库字段（partial=true 时允许缺省字段，用于 PATCH）。
func (in ruleInput) normalize(partial bool) (map[string]any, error) {
	upd := map[string]any{}

	name := strings.TrimSpace(in.Name)
	if name != "" {
		upd["name"] = name
	} else if !partial {
		upd["name"] = "未命名规则"
	}

	if e := strings.TrimSpace(in.Expression); e != "" {
		if err := rules.Validate(e); err != nil {
			return nil, err
		}
		upd["expression"] = e
	} else if !partial {
		return nil, errText("表达式不能为空")
	}

	if in.Enabled != nil {
		upd["enabled"] = *in.Enabled
	} else if !partial {
		upd["enabled"] = true
	}
	if in.Priority != nil {
		upd["priority"] = *in.Priority
	} else if !partial {
		upd["priority"] = 0
	}

	if in.Action != "" {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if !rules.AllowedActions[action] {
			return nil, errText("不支持的动作（trash/move）")
		}
		upd["action"] = action
		folder := strings.TrimSpace(in.Folder)
		if action == "move" {
			if !rules.AllowedFolders[folder] {
				return nil, errText("目标文件夹非法（inbox/draft/trash）")
			}
			upd["folder"] = folder
		} else {
			upd["folder"] = ""
		}
	} else if !partial {
		upd["action"] = "trash"
		upd["folder"] = ""
	}
	return upd, nil
}

type errText string

func (e errText) Error() string { return string(e) }

// GET/POST /api/rules  或  /api/admin/rules
func (h *RuleBox) List(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var rs []model.MailRule
		h.DB.Where("user_id = ?", owner).Order("priority DESC, id ASC").Find(&rs)
		if rs == nil {
			rs = []model.MailRule{}
		}
		writeJSON(w, 200, rs)
	case "POST":
		var in ruleInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd, err := in.normalize(false)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		rule := model.MailRule{UserID: owner}
		rule.Name, _ = upd["name"].(string)
		rule.Expression, _ = upd["expression"].(string)
		rule.Action, _ = upd["action"].(string)
		rule.Folder, _ = upd["folder"].(string)
		rule.Enabled, _ = upd["enabled"].(bool)
		rule.Priority, _ = upd["priority"].(int)
		if err := h.DB.Create(&rule).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, rule)
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/rules/{id}  或  /api/admin/rules/{id}
func (h *RuleBox) One(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.owner(w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/rules/")
	if h.Site {
		rest = strings.TrimPrefix(r.URL.Path, "/api/admin/rules/")
	}
	id := strings.Split(rest, "/")[0]
	var rule model.MailRule
	if err := h.DB.Where("id = ? AND user_id = ?", id, owner).First(&rule).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&rule)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in ruleInput
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
			h.DB.Model(&rule).Updates(upd)
		}
		h.DB.Where("id = ? AND user_id = ?", id, owner).First(&rule)
		writeJSON(w, 200, rule)
	default:
		w.WriteHeader(405)
	}
}

// POST /api/rules/test 或 /api/admin/rules/test：用示例邮件测试表达式
func (h *RuleBox) Test(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.owner(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Expression  string `json:"expression"`
		From        string `json:"from"`
		To          string `json:"to"`
		Cc          string `json:"cc"`
		Bcc         string `json:"bcc"`
		Subject     string `json:"subject"`
		Body        string `json:"body"`
		Size        int    `json:"size"`
		Attachments int    `json:"attachments"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	matched, err := rules.Match(in.Expression, rules.Input{
		From: in.From, To: in.To, Cc: in.Cc, Bcc: in.Bcc,
		Subject: in.Subject, Body: in.Body, Size: in.Size, Attachments: in.Attachments,
	})
	if err != nil {
		writeJSON(w, 200, map[string]any{"matched": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"matched": matched})
}
