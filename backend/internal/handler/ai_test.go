package handler

import (
	"testing"

	"mailserver/internal/ai"
	"mailserver/internal/model"
	"mailserver/internal/secret"
)

func TestApplyAIInputCreateDefaults(t *testing.T) {
	secret.SetKey("ai-test")
	p := model.AIProvider{Scope: ai.ScopeUser, UserID: 7}
	if err := applyAIInput(&p, aiInput{Provider: "openai"}, false); err != nil {
		t.Fatalf("内置服务商不应报错：%v", err)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("应回退默认 BaseURL，得到 %q", p.BaseURL)
	}
	if p.Name != "OpenAI" {
		t.Fatalf("应回退服务商标签，得到 %q", p.Name)
	}
	if !p.Enabled {
		t.Fatal("新建默认应启用")
	}
}

func TestApplyAIInputCustomRequiresBaseURL(t *testing.T) {
	p := model.AIProvider{}
	if err := applyAIInput(&p, aiInput{Provider: "custom"}, false); err == nil {
		t.Fatal("custom 缺少 BaseURL 应报错")
	}
}

func TestApplyAIInputUnknownProvider(t *testing.T) {
	p := model.AIProvider{}
	if err := applyAIInput(&p, aiInput{Provider: "mystery"}, false); err == nil {
		t.Fatal("未知服务商应报错")
	}
}

func TestApplyAIInputEncryptsKeyAndPartialKeeps(t *testing.T) {
	secret.SetKey("ai-test")
	p := model.AIProvider{}
	if err := applyAIInput(&p, aiInput{Provider: "deepseek", APIKey: "sk-1"}, false); err != nil {
		t.Fatal(err)
	}
	enc := p.APIKey
	if enc == "" || enc == "sk-1" {
		t.Fatal("API Key 应被加密存储")
	}
	dec, err := secret.Decrypt(enc)
	if err != nil || string(dec) != "sk-1" {
		t.Fatalf("解密应还原：%v %q", err, dec)
	}
	// PATCH 未传 key 时保留原密文
	p2 := model.AIProvider{Provider: "deepseek", APIKey: enc, Enabled: true}
	if err := applyAIInput(&p2, aiInput{Model: "deepseek-chat"}, true); err != nil {
		t.Fatal(err)
	}
	if p2.APIKey != enc {
		t.Fatal("部分更新未传 key 应保留原值")
	}
	// 部分更新不应把 Enabled 置为 false
	if !p2.Enabled {
		t.Fatal("部分更新不应改动未提供的 Enabled")
	}
}

func TestApplyAIInputDisable(t *testing.T) {
	secret.SetKey("ai-test")
	off := false
	p := model.AIProvider{}
	if err := applyAIInput(&p, aiInput{Provider: "openai", Enabled: &off}, false); err != nil {
		t.Fatal(err)
	}
	if p.Enabled {
		t.Fatal("显式 Enabled=false 应生效")
	}
}
