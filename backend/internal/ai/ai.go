// Package ai 提供 AI 服务商目录、BYOK（Bring Your Own Key）配置解析与连通性测试，
// 为后续 AI 特性（摘要、分类、智能回复、语义检索等）打基础。
//
// 配置分两级：用户级（用户自有 Key）优先，系统级（管理员配置的整站默认）兜底。
package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/secret"

	"gorm.io/gorm"
)

// Scope 取值。
const (
	ScopeSystem = "system"
	ScopeUser   = "user"
)

// Provider 描述一个内置的 AI 服务商（OpenAI 兼容协议为主）。
type Provider struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	BaseURL  string   `json:"base_url"`
	Models   []string `json:"models"`
	KeyLabel string   `json:"key_label"`
	Local    bool     `json:"local"` // 本地服务（无需 Key）
}

// catalog 内置服务商目录。BaseURL 统一填到含版本号的根，客户端在其后拼 /chat/completions 等。
var catalog = []Provider{
	{
		ID: "openai", Label: "OpenAI",
		BaseURL:  "https://api.openai.com/v1",
		Models:   []string{"gpt-4o", "gpt-4o-mini", "gpt-4.1", "gpt-4.1-mini", "o3-mini"},
		KeyLabel: "API Key",
	},
	{
		ID: "anthropic", Label: "Anthropic Claude",
		BaseURL:  "https://api.anthropic.com/v1",
		Models:   []string{"claude-sonnet-4-5", "claude-opus-4-1", "claude-3-5-haiku-latest"},
		KeyLabel: "API Key",
	},
	{
		ID: "deepseek", Label: "DeepSeek",
		BaseURL:  "https://api.deepseek.com/v1",
		Models:   []string{"deepseek-chat", "deepseek-reasoner"},
		KeyLabel: "API Key",
	},
	{
		ID: "moonshot", Label: "Moonshot / Kimi",
		BaseURL:  "https://api.moonshot.cn/v1",
		Models:   []string{"moonshot-v1-8k", "moonshot-v1-32k", "kimi-k2-0905-preview"},
		KeyLabel: "API Key",
	},
	{
		ID: "dashscope", Label: "阿里云百炼 / 通义千问",
		BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Models:   []string{"qwen-plus", "qwen-turbo", "qwen-max", "qwen2.5-72b-instruct"},
		KeyLabel: "DashScope API Key",
	},
	{
		ID: "zhipu", Label: "智谱 GLM",
		BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
		Models:   []string{"glm-4-plus", "glm-4-flash", "glm-4-air"},
		KeyLabel: "API Key",
	},
	{
		ID: "openrouter", Label: "OpenRouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Models:   []string{"openai/gpt-4o-mini", "anthropic/claude-sonnet-4", "google/gemini-2.0-flash-001"},
		KeyLabel: "API Key",
	},
	{
		ID: "ollama", Label: "Ollama（本地）",
		BaseURL:  "http://localhost:11434/v1",
		Models:   []string{"llama3.2", "qwen2.5", "mistral"},
		KeyLabel: "API Key（可留空）",
		Local:    true,
	},
	{
		ID: "custom", Label: "兼容 OpenAI 的自定义端点",
		BaseURL:  "",
		Models:   []string{},
		KeyLabel: "API Key",
	},
}

// Catalog 返回内置服务商目录副本。
func Catalog() []Provider {
	out := make([]Provider, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup 按 ID 查找服务商（大小写不敏感）。
func Lookup(id string) (Provider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// NormalizeBaseURL 返回显式 BaseURL（去尾斜杠）；为空时回退目录默认值。
func NormalizeBaseURL(providerID, explicit string) string {
	if b := strings.TrimRight(strings.TrimSpace(explicit), "/"); b != "" {
		return b
	}
	if p, ok := Lookup(providerID); ok {
		return p.BaseURL
	}
	return ""
}

// Config 是解析后的可用配置（APIKey 为明文，仅服务端使用）。
type Config struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

// Effective 表示针对某个用户解析出的配置及其来源。
type Effective struct {
	Config
	Source  string // user | system
	Enabled bool
}

// ApplyBaseURL 补全 BaseURL。
func (c Config) resolved() Config {
	c.BaseURL = NormalizeBaseURL(c.Provider, c.BaseURL)
	return c
}

// Resolve 解析用户可用的 AI 配置：优先用户级 BYOK，回退系统级默认；都没有则 ok=false。
// 解密失败的行会被跳过并继续尝试下一行，保证单个坏配置不阻塞整体。
func Resolve(db *gorm.DB, userID uint) (Effective, bool) {
	if db == nil {
		return Effective{}, false
	}
	for _, scope := range []string{ScopeUser, ScopeSystem} {
		q := db.Where("scope = ? AND enabled = ?", scope, true)
		if scope == ScopeUser {
			q = q.Where("user_id = ?", userID)
		}
		var rows []model.AIProvider
		if err := q.Order("id ASC").Find(&rows).Error; err != nil {
			continue
		}
		for _, row := range rows {
			key, err := secret.Decrypt(row.APIKey)
			if err != nil {
				continue
			}
			cfg := Config{Provider: row.Provider, BaseURL: row.BaseURL, Model: row.Model, APIKey: string(key)}
			return Effective{Config: cfg.resolved(), Source: scope, Enabled: true}, true
		}
	}
	return Effective{}, false
}

// Test 通过调用服务商的 /models 端点验证连通性与凭证。
// OpenAI、Anthropic 及绝大多数兼容端点均支持该接口，且不消耗推理额度。
func Test(ctx context.Context, cfg Config) error {
	cfg = cfg.resolved()
	if cfg.BaseURL == "" {
		return errors.New("缺少 Base URL")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("Base URL 需为 http(s) 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	if cfg.Provider == "anthropic" {
		req.Header.Set("x-api-key", cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d：%s", resp.StatusCode, msg)
}
