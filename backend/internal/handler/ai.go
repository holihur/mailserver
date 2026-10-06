package handler

// AI BYOK（Bring Your Own Key）配置管理。
//
// 分两级：
//   - 用户级：/api/ai/providers（GET 列表 / POST 新建）、/api/ai/providers/{id}（PATCH/DELETE）、
//     /api/ai/providers/{id}/test（测试连通性）。仅能操作自己的配置。
//   - 系统级：/api/admin/ai-providers 及其子路径，仅管理员可维护，作为整站默认。
//
// API Key 用 AES-GCM 加密落库，接口不回传明文，仅回传 api_key_set 布尔标记。
// 解析优先级：用户级 > 系统级（见 internal/ai.Resolve）。

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/ai"
	"mailserver/internal/model"
	"mailserver/internal/secret"

	"gorm.io/gorm"
)

// AIBox 用户级 BYOK 配置。
type AIBox struct{ DB *gorm.DB }

type aiView struct {
	ID            uint      `json:"id"`
	Scope         string    `json:"scope"`
	UserID        uint      `json:"user_id"`
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	ProviderLabel string    `json:"provider_label"`
	BaseURL       string    `json:"base_url"`
	Model         string    `json:"model"`
	APIKeySet     bool      `json:"api_key_set"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type aiInput struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	Enabled  *bool  `json:"enabled"`
}

func viewOfAI(p model.AIProvider) aiView {
	label := p.Provider
	if c, ok := ai.Lookup(p.Provider); ok {
		label = c.Label
	}
	return aiView{
		ID: p.ID, Scope: p.Scope, UserID: p.UserID, Name: p.Name,
		Provider: p.Provider, ProviderLabel: label,
		BaseURL: ai.NormalizeBaseURL(p.Provider, p.BaseURL), Model: p.Model,
		APIKeySet: p.APIKey != "", Enabled: p.Enabled,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// applyAIInput 校验并填充配置；partial=true 时允许缺省字段（用于 PATCH）。
func applyAIInput(p *model.AIProvider, in aiInput, partial bool) error {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider != "" {
		if _, ok := ai.Lookup(provider); !ok {
			return errText("不支持的服务商类型")
		}
		// 服务商变更且未显式给 BaseURL 时，改用新服务商默认值，避免沿用旧地址
		if provider != p.Provider && strings.TrimSpace(in.BaseURL) == "" {
			p.BaseURL = ai.NormalizeBaseURL(provider, "")
		}
		p.Provider = provider
	} else if !partial {
		return errText("请选择服务商类型")
	}

	if b := strings.TrimSpace(in.BaseURL); b != "" {
		p.BaseURL = strings.TrimRight(b, "/")
	}
	if p.BaseURL == "" {
		p.BaseURL = ai.NormalizeBaseURL(p.Provider, "")
	}
	if p.Provider == "custom" && ai.NormalizeBaseURL(p.Provider, p.BaseURL) == "" {
		return errText("自定义端点必须填写 Base URL")
	}

	if m := strings.TrimSpace(in.Model); m != "" {
		p.Model = m
	}
	if name := strings.TrimSpace(in.Name); name != "" {
		if len([]rune(name)) > 60 {
			name = string([]rune(name)[:60])
		}
		p.Name = name
	}
	if p.Name == "" {
		if c, ok := ai.Lookup(p.Provider); ok {
			p.Name = c.Label
		} else {
			p.Name = p.Provider
		}
	}

	if k := strings.TrimSpace(in.APIKey); k != "" {
		enc, err := secret.Encrypt([]byte(k))
		if err != nil {
			return errText("API Key 加密失败")
		}
		p.APIKey = enc
	}

	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	} else if !partial {
		p.Enabled = true
	}
	return nil
}

func listAIProviders(db *gorm.DB, scope string, uid uint) []aiView {
	q := db.Where("scope = ?", scope)
	if scope == ai.ScopeUser {
		q = q.Where("user_id = ?", uid)
	}
	var rows []model.AIProvider
	q.Order("id ASC").Find(&rows)
	out := make([]aiView, 0, len(rows))
	for _, row := range rows {
		out = append(out, viewOfAI(row))
	}
	return out
}

func createAIProvider(db *gorm.DB, scope string, uid uint, w http.ResponseWriter, r *http.Request) {
	var in aiInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	p := model.AIProvider{Scope: scope, UserID: uid}
	if err := applyAIInput(&p, in, false); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := db.Create(&p).Error; err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存失败"})
		return
	}
	writeJSON(w, 201, viewOfAI(p))
}

func updateAIProvider(db *gorm.DB, scope string, uid uint, id uint, w http.ResponseWriter, r *http.Request) {
	p, ok := findAIProvider(db, scope, uid, id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "配置不存在"})
		return
	}
	var in aiInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	if err := applyAIInput(&p, in, true); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := db.Save(&p).Error; err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存失败"})
		return
	}
	writeJSON(w, 200, viewOfAI(p))
}

func findAIProvider(db *gorm.DB, scope string, uid, id uint) (model.AIProvider, bool) {
	var p model.AIProvider
	q := db.Where("id = ? AND scope = ?", id, scope)
	if scope == ai.ScopeUser {
		q = q.Where("user_id = ?", uid)
	}
	if err := q.First(&p).Error; err != nil {
		return model.AIProvider{}, false
	}
	return p, true
}

// testAIProvider 读取已存配置（解密 Key）并调用服务商 /models 验证连通性。
func testAIProvider(db *gorm.DB, scope string, uid, id uint, w http.ResponseWriter, r *http.Request) {
	p, ok := findAIProvider(db, scope, uid, id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "配置不存在"})
		return
	}
	key, err := secret.Decrypt(p.APIKey)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "API Key 解密失败（JWT_SECRET 是否变更过？）"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cfg := ai.Config{Provider: p.Provider, BaseURL: p.BaseURL, Model: p.Model, APIKey: string(key)}
	if err := ai.Test(ctx, cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": "连接失败：" + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ---- 用户级路由 ----

// GET /api/ai/providers  POST /api/ai/providers
func (h *AIBox) Providers(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, listAIProviders(h.DB, ai.ScopeUser, uid))
	case "POST":
		createAIProvider(h.DB, ai.ScopeUser, uid, w, r)
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/ai/providers/{id}    POST /api/ai/providers/{id}/test
func (h *AIBox) ProviderOne(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ai/providers/"), "/")
	parts := strings.Split(rest, "/")
	id64, _ := strconv.ParseUint(parts[0], 10, 32)
	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		testAIProvider(h.DB, ai.ScopeUser, uid, uint(id64), w, r)
		return
	}
	if len(parts) != 1 {
		w.WriteHeader(404)
		return
	}
	switch r.Method {
	case "PATCH":
		updateAIProvider(h.DB, ai.ScopeUser, uid, uint(id64), w, r)
	case "DELETE":
		p, ok := findAIProvider(h.DB, ai.ScopeUser, uid, uint(id64))
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "配置不存在"})
			return
		}
		h.DB.Delete(&p)
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(405)
	}
}

// GET /api/ai/catalog -> 内置服务商目录 + 当前生效来源
func (h *AIBox) Catalog(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	writeJSON(w, 200, map[string]any{
		"providers": ai.Catalog(),
		"status":    aiStatus(h.DB, uid),
	})
}

type aiStatusView struct {
	HasUserKey      bool   `json:"has_user_key"`
	HasSystemKey    bool   `json:"has_system_key"`
	EffectiveSource string `json:"effective_source"` // user | system | ""
	Enabled         bool   `json:"enabled"`
}

func aiStatus(db *gorm.DB, uid uint) aiStatusView {
	var own, sys int64
	db.Model(&model.AIProvider{}).Where("scope = ? AND user_id = ? AND enabled = ?", ai.ScopeUser, uid, true).Count(&own)
	db.Model(&model.AIProvider{}).Where("scope = ? AND enabled = ?", ai.ScopeSystem, true).Count(&sys)
	st := aiStatusView{HasUserKey: own > 0, HasSystemKey: sys > 0}
	switch {
	case st.HasUserKey:
		st.EffectiveSource, st.Enabled = ai.ScopeUser, true
	case st.HasSystemKey:
		st.EffectiveSource, st.Enabled = ai.ScopeSystem, true
	}
	return st
}

// GET /api/ai/status -> 用户/系统是否已配置可用 Key（供界面提示）
func (h *AIBox) Status(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	writeJSON(w, 200, aiStatus(h.DB, uid))
}

// ---- 系统级路由（管理员） ----

// GET /api/admin/ai-providers  POST /api/admin/ai-providers
func (a *Admin) AIProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, listAIProviders(a.DB, ai.ScopeSystem, 0))
	case "POST":
		createAIProvider(a.DB, ai.ScopeSystem, 0, w, r)
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/admin/ai-providers/{id}    POST /api/admin/ai-providers/{id}/test
func (a *Admin) AIProviderOne(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/ai-providers/"), "/")
	parts := strings.Split(rest, "/")
	id64, _ := strconv.ParseUint(parts[0], 10, 32)
	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		testAIProvider(a.DB, ai.ScopeSystem, 0, uint(id64), w, r)
		return
	}
	if len(parts) != 1 {
		w.WriteHeader(404)
		return
	}
	switch r.Method {
	case "PATCH":
		updateAIProvider(a.DB, ai.ScopeSystem, 0, uint(id64), w, r)
	case "DELETE":
		p, ok := findAIProvider(a.DB, ai.ScopeSystem, 0, uint(id64))
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "配置不存在"})
			return
		}
		a.DB.Delete(&p)
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(405)
	}
}
