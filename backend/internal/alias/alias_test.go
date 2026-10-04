package alias

import (
	"os"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

func TestTargets(t *testing.T) {
	got := Targets("D@x.com, e@y.com;d@x.com  f@g.com")
	want := []string{"d@x.com", "e@y.com", "f@g.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if len(Targets("not-an-email, ,")) != 0 {
		t.Fatal("非法目标应被过滤")
	}
}

func TestValidSource(t *testing.T) {
	for _, s := range []string{"abc@example.com", "@example.com"} {
		if !ValidSource(s) {
			t.Errorf("应合法: %q", s)
		}
	}
	for _, s := range []string{"", "abc", "@", "a b@x.com", "@nodot"} {
		if ValidSource(s) {
			t.Errorf("应非法: %q", s)
		}
	}
}

func TestMatchDB(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE mail_aliases RESTART IDENTITY")
	g.Create(&model.MailAlias{Source: "abc@example.com", Targets: "d@example.com", Enabled: true})
	g.Create(&model.MailAlias{Source: "@example.com", Targets: "catch@example.com", Enabled: true})
	g.Create(&model.MailAlias{Source: "off@off.net", Targets: "x@y.z", Enabled: false})

	// 精确匹配（大小写不敏感）
	if m := Match(g, "ABC@example.com"); m == nil || m.Source != "abc@example.com" {
		t.Fatalf("精确匹配失败: %+v", m)
	}
	// 整域 catch-all
	if m := Match(g, "other@example.com"); m == nil || m.Source != "@example.com" {
		t.Fatalf("catch-all 失败: %+v", m)
	}
	// 禁用不匹配
	if m := Match(g, "off@off.net"); m != nil {
		t.Fatalf("禁用别名不应匹配: %+v", m)
	}
	// 无匹配
	if m := Match(g, "x@other.net"); m != nil {
		t.Fatalf("无匹配应为 nil: %+v", m)
	}
}
