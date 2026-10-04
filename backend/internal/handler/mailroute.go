package handler

// 邮件路由（管理员）：/api/admin/routes（GET 列表 / POST 新建）、/api/admin/routes/{id}（PATCH/DELETE）。
// 按收件人域名把外发邮件交给指定中继 / 直连 / 丢弃；中继密码加密存储。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/model"
	"mailserver/internal/route"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/secret"

	"gorm.io/gorm"
)

type RouteBox struct {
	DB          *gorm.DB
	RT          *runtimecfg.Store
	AdminEmails string
}

type routeView struct {
	model.MailRoute
	RelayPassSet bool `json:"relay_pass_set"`
}

type routeInput struct {
	Domain    string `json:"domain"`
	Action    string `json:"action"`
	RelayHost string `json:"relay_host"`
	RelayPort string `json:"relay_port"`
	RelayUser string `json:"relay_user"`
	RelayPass string `json:"relay_pass"`
	RelayFrom string `json:"relay_from"`
	Insecure  *bool  `json:"insecure"`
	Priority  *int   `json:"priority"`
	Enabled   *bool  `json:"enabled"`
}

func (in routeInput) build(existing *model.MailRoute) (*model.MailRoute, error) {
	r := model.MailRoute{}
	if existing != nil {
		r = *existing
	}
	r.Domain = strings.ToLower(strings.Trim(strings.TrimSpace(in.Domain), "."))
	if r.Domain == "" || strings.ContainsAny(r.Domain, " /") {
		return nil, errText("域名不能为空或非法")
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		action = "relay"
	}
	if action != "relay" && action != "direct" && action != "discard" {
		return nil, errText("动作非法（relay/direct/discard）")
	}
	r.Action = action
	r.RelayHost = strings.TrimSpace(in.RelayHost)
	r.RelayPort = strings.TrimSpace(in.RelayPort)
	r.RelayUser = strings.TrimSpace(in.RelayUser)
	r.RelayFrom = strings.TrimSpace(in.RelayFrom)
	r.Insecure = in.Insecure != nil && *in.Insecure
	r.Priority = 0
	if in.Priority != nil {
		r.Priority = *in.Priority
	}
	r.Enabled = true
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	if action == "relay" && r.RelayHost == "" {
		return nil, errText("relay 动作需要填写中继服务器")
	}
	if in.RelayPass != "" {
		enc, err := secret.Encrypt([]byte(in.RelayPass))
		if err != nil {
			return nil, errText("密码加密失败")
		}
		r.RelayPass = enc
	} else if existing == nil {
		r.RelayPass = ""
	}
	return &r, nil
}

// GET/POST /api/admin/routes
func (h *RouteBox) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		var rs []model.MailRoute
		h.DB.Order("priority DESC, domain ASC").Find(&rs)
		out := make([]routeView, 0, len(rs))
		for _, x := range rs {
			out = append(out, routeView{MailRoute: x, RelayPassSet: x.RelayPass != ""})
		}
		writeJSON(w, 200, out)
	case "POST":
		var in routeInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		nr, err := in.build(nil)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := h.DB.Create(nr).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, routeView{MailRoute: *nr, RelayPassSet: nr.RelayPass != ""})
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/admin/routes/{id}
func (h *RouteBox) One(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/routes/")
	id = strings.Split(id, "/")[0]
	var rt model.MailRoute
	if err := h.DB.Where("id = ?", id).First(&rt).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Delete(&rt)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in routeInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		nr, err := in.build(&rt)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		nr.ID = rt.ID
		if err := h.DB.Save(nr).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 200, routeView{MailRoute: *nr, RelayPassSet: nr.RelayPass != ""})
	default:
		w.WriteHeader(405)
	}
}

// POST /api/admin/routes/test {domain} -> 预览命中的路由
func (h *RouteBox) Test(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveAdmin(h.DB, h.RT, h.AdminEmails, w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Domain string `json:"domain"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	var rs []model.MailRoute
	h.DB.Find(&rs)
	m := route.Match(rs, in.Domain)
	if m == nil {
		writeJSON(w, 200, map[string]any{"matched": false})
		return
	}
	writeJSON(w, 200, map[string]any{"matched": true, "route": routeView{MailRoute: *m, RelayPassSet: m.RelayPass != ""}})
}
