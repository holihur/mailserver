package route

import (
	"testing"

	"mailserver/internal/model"
)

func TestMatch(t *testing.T) {
	routes := []model.MailRoute{
		{Domain: "example.com", Action: "relay", Enabled: true},
		{Domain: ".example.com", Action: "direct", Enabled: true},
		{Domain: "sub.example.com", Action: "discard", Enabled: true},
		{Domain: "off.com", Action: "relay", Enabled: false},
	}
	// 精确优先于后缀
	if m := Match(routes, "example.com"); m == nil || m.Action != "relay" {
		t.Fatalf("精确匹配失败: %+v", m)
	}
	// 更具体的精确子域优先
	if m := Match(routes, "sub.example.com"); m == nil || m.Action != "discard" {
		t.Fatalf("子域精确匹配失败: %+v", m)
	}
	// 后缀匹配
	if m := Match(routes, "mail.example.com"); m == nil || m.Action != "direct" {
		t.Fatalf("后缀匹配失败: %+v", m)
	}
	// 未启用不匹配
	if m := Match(routes, "off.com"); m != nil {
		t.Fatalf("禁用路由不应命中: %+v", m)
	}
	// 无匹配
	if m := Match(routes, "other.net"); m != nil {
		t.Fatalf("无匹配应为 nil: %+v", m)
	}
}

func TestPriority(t *testing.T) {
	routes := []model.MailRoute{
		{Domain: ".example.com", Action: "direct", Enabled: true, Priority: 1},
		{Domain: ".example.com", Action: "discard", Enabled: true, Priority: 5},
	}
	if m := Match(routes, "a.example.com"); m == nil || m.Action != "discard" {
		t.Fatalf("高优先级应胜出: %+v", m)
	}
}
