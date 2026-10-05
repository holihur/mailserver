package handler

// CEL 收信规则：用户级 /api/rules，整站级 /api/admin/rules（仅管理员）。
// 命中后把邮件投递到指定文件夹（默认 trash）。表达式返回 bool。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/deliver"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/push"
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
	Shadow     *bool  `json:"shadow"`
	Priority   *int   `json:"priority"`
	Expression string `json:"expression"`
	Action     string `json:"action"`
	Folder     string `json:"folder"`
	ForwardTo  string `json:"forward_to"`
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
	if in.Shadow != nil {
		upd["shadow"] = *in.Shadow
	} else if !partial {
		upd["shadow"] = false
	}
	if in.Priority != nil {
		upd["priority"] = *in.Priority
	} else if !partial {
		upd["priority"] = 0
	}

	if in.Action != "" {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if !rules.AllowedActions[action] {
			return nil, errText("不支持的动作（trash/move/forward）")
		}
		upd["action"] = action
		switch action {
		case "move":
			folder := strings.TrimSpace(in.Folder)
			if folder == "" {
				return nil, errText("请选择目标文件夹")
			}
			upd["folder"] = folder
			upd["forward_to"] = ""
		case "forward":
			targets := rules.SplitTargets(in.ForwardTo)
			if len(targets) == 0 {
				return nil, errText("转发动作需要填写目标邮箱")
			}
			upd["forward_to"] = strings.Join(targets, ", ")
			upd["folder"] = ""
		default:
			upd["folder"] = ""
			upd["forward_to"] = ""
		}
	} else if !partial {
		upd["action"] = "trash"
		upd["folder"] = ""
		upd["forward_to"] = ""
	}
	return upd, nil
}

type errText string

func (e errText) Error() string { return string(e) }

// apply 把单条规则回放到已有邮件（对旧邮件生效）。dry_run=true 仅预览匹配；
// 规则为影子模式时不执行动作。整站规则会回放所有用户的邮件。
func (h *RuleBox) apply(w http.ResponseWriter, r *http.Request, rule *model.MailRule, owner uint) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Folder string `json:"folder"`
		Limit  int    `json:"limit"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	folder := strings.TrimSpace(in.Folder)
	if folder == "" {
		folder = "inbox"
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	tx := h.DB.Where("folder = ?", folder)
	if !h.Site {
		tx = tx.Where("user_id = ?", owner)
	}
	var mails []model.Mail
	tx.Order("id DESC").Limit(limit).Find(&mails)

	matched, applied := 0, 0
	samples := []map[string]any{}
	notify := map[uint]bool{}
	for i := range mails {
		mail := &mails[i]
		ok, err := rules.Match(rule.Expression, rules.Input{
			From: mail.From, To: mail.To, Cc: mail.Cc, Bcc: mail.Bcc,
			Subject: mail.Subject, Body: mail.Body,
			Size: len(mail.Body), Attachments: len(message.ParseAttachments(mail.Attachments)),
		})
		if err != nil || !ok {
			continue
		}
		matched++
		if len(samples) < 20 {
			samples = append(samples, map[string]any{"id": mail.ID, "subject": mail.Subject, "from": mail.From})
		}
		if in.DryRun || rule.Shadow {
			continue
		}
		switch rule.Action {
		case "move":
			h.DB.Model(&model.Mail{}).Where("id = ?", mail.ID).Update("folder", rule.Folder)
		case "trash":
			h.DB.Model(&model.Mail{}).Where("id = ?", mail.ID).Update("folder", "trash")
		case "forward":
			if targets := rules.SplitTargets(rule.ForwardTo); len(targets) > 0 {
				deliver.Forward(h.DB, mail.From, targets, mail.Subject, mail.Body, mail.Attachments)
			}
		}
		applied++
		notify[mail.UserID] = true
	}
	for uid := range notify {
		push.Notify(uid)
	}
	writeJSON(w, 200, map[string]any{
		"matched": matched, "applied": applied, "samples": samples, "shadow": rule.Shadow,
	})
}

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
		if f, ok := upd["folder"].(string); ok && f != "" && !FolderValid(h.DB, owner, f) {
			writeJSON(w, 400, map[string]string{"error": "目标文件夹非法"})
			return
		}
		rule := model.MailRule{UserID: owner}
		rule.Name, _ = upd["name"].(string)
		rule.Expression, _ = upd["expression"].(string)
		rule.Action, _ = upd["action"].(string)
		rule.Folder, _ = upd["folder"].(string)
		rule.ForwardTo, _ = upd["forward_to"].(string)
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
	// POST /api/rules/{id}/apply：把规则回放到已有邮件（dry_run 仅预览）
	if strings.HasSuffix(rest, "/apply") {
		h.apply(w, r, &rule, owner)
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
		if f, ok := upd["folder"].(string); ok && f != "" && !FolderValid(h.DB, owner, f) {
			writeJSON(w, 400, map[string]string{"error": "目标文件夹非法"})
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
