package rules

import (
	"testing"

	"mailserver/internal/model"
)

func TestMatch(t *testing.T) {
	in := Input{From: "spam@bad.com", To: "me@x.com", Subject: "Cheap Viagra", Body: "buy now", Size: 1234, Attachments: 1}
	cases := []struct {
		expr string
		want bool
	}{
		{`from.endsWith("@bad.com")`, true},
		{`subject.contains("Viagra")`, true},
		{`subject.contains("viagra")`, false},
		{`size > 1000`, true},
		{`attachments > 0`, true},
		{`from == "spam@bad.com" && to.endsWith("@x.com")`, true},
		{`subject.matches("(?i)viagra")`, true},
		{`to.contains("nobody")`, false},
	}
	for _, c := range cases {
		got, err := Match(c.expr, in)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("%s = %v want %v", c.expr, got, c.want)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	if err := Validate(`size + 1`); err == nil {
		t.Fatal("非布尔表达式应被拒绝")
	}
	if err := Validate(`from.`); err == nil {
		t.Fatal("语法错误应被拒绝")
	}
	if err := Validate(""); err == nil {
		t.Fatal("空表达式应被拒绝")
	}
	if err := Validate(`from.contains("x")`); err != nil {
		t.Fatalf("合法表达式不应报错: %v", err)
	}
}

func TestEvaluateOrder(t *testing.T) {
	// 整站规则优先于用户规则
	list := []model.MailRule{
		{ID: 1, UserID: 5, Name: "user", Enabled: true, Priority: 99, Expression: `subject.contains("x")`, Action: "trash"},
		{ID: 2, UserID: 0, Name: "site", Enabled: true, Expression: `subject.contains("x")`, Action: "move", Folder: "draft"},
	}
	d := Evaluate(list, Input{Subject: "x"})
	if !d.Matched || d.Scope != "site" || d.Folder != "draft" {
		t.Fatalf("整站规则应优先: %+v", d)
	}

	// 整站不命中时用户规则生效
	list2 := []model.MailRule{
		{ID: 1, UserID: 5, Name: "user", Enabled: true, Expression: `subject.contains("x")`, Action: "trash"},
		{ID: 2, UserID: 0, Name: "site", Enabled: true, Expression: `subject.contains("zzz")`, Action: "trash"},
	}
	d2 := Evaluate(list2, Input{Subject: "x"})
	if !d2.Matched || d2.Scope != "user" || d2.Folder != "trash" {
		t.Fatalf("用户规则应生效: %+v", d2)
	}

	// 未命中
	if Evaluate(list2, Input{Subject: "hello"}).Matched {
		t.Fatal("不应命中")
	}
	// 禁用规则不参与
	off := []model.MailRule{{UserID: 0, Enabled: false, Expression: `subject.contains("x")`, Action: "trash"}}
	if Evaluate(off, Input{Subject: "x"}).Matched {
		t.Fatal("禁用规则不应命中")
	}
}

func TestActionFolder(t *testing.T) {
	if got := actionFolder(model.MailRule{Action: "trash"}); got != "trash" {
		t.Fatalf("trash=%s", got)
	}
	if got := actionFolder(model.MailRule{Action: "move", Folder: "inbox"}); got != "inbox" {
		t.Fatalf("move=%s", got)
	}
	if got := actionFolder(model.MailRule{Action: "move", Folder: "sent"}); got != "trash" {
		t.Fatalf("非法目标应回退 trash，得到 %s", got)
	}
}
