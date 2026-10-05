package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

// 规则回放：dry_run 仅预览，应用后对已有邮件生效（#收信规则对旧邮件）。
func TestRuleApplyReplay(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails, mail_rules RESTART IDENTITY CASCADE")
	auth.SetSecret("unit-test-secret-0123456789")
	u := model.User{Email: "ra@test.local"}
	g.Create(&u)
	for i := 0; i < 3; i++ {
		g.Create(&model.Mail{UserID: u.ID, From: "x@y.z", To: u.Email, Subject: "促销活动", Body: "buy", Folder: "inbox"})
	}
	g.Create(&model.Mail{UserID: u.ID, From: "a@b.c", To: u.Email, Subject: "正常", Body: "hi", Folder: "inbox"})
	rule := model.MailRule{UserID: u.ID, Name: "promo", Enabled: true, Expression: `subject.contains("促销")`, Action: "trash"}
	g.Create(&rule)

	tok, _ := auth.Sign(u.ID, u.Email, 0)
	rb := &RuleBox{DB: g}
	apply := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/rules/"+strconv.Itoa(int(rule.ID))+"/apply", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		rb.One(rr, req)
		return rr
	}
	var out struct{ Matched, Applied int }

	// 预览：匹配 3 封，但不改动
	rr := apply(`{"folder":"inbox","limit":100,"dry_run":true}`)
	if rr.Code != 200 {
		t.Fatalf("预览应 200，得到 %d %s", rr.Code, rr.Body.String())
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Matched != 3 || out.Applied != 0 {
		t.Fatalf("预览应 matched=3 applied=0，得到 %+v", out)
	}
	var n int64
	g.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, "trash").Count(&n)
	if n != 0 {
		t.Fatalf("预览不应改动，trash 有 %d 封", n)
	}

	// 应用：3 封进垃圾箱
	rr = apply(`{"folder":"inbox","limit":100}`)
	if rr.Code != 200 {
		t.Fatalf("应用应 200，得到 %d %s", rr.Code, rr.Body.String())
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Applied != 3 {
		t.Fatalf("应处理 3 封，得到 %+v", out)
	}
	g.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, "trash").Count(&n)
	if n != 3 {
		t.Fatalf("应有 3 封进垃圾箱，得到 %d", n)
	}
}
