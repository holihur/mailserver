package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #48：会话聚合按规范化主题分组（去 Re/Fwd/回复 前缀）。
func TestListThreads(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails RESTART IDENTITY CASCADE")
	auth.SetSecret("unit-test-secret-0123456789")
	u := model.User{Email: "th@test.local"}
	g.Create(&u)
	for _, s := range []string{"Hello", "Re: Hello", "Fwd: Re: Hello", "回复: Hello"} {
		g.Create(&model.Mail{UserID: u.ID, From: "a@x", To: u.Email, Subject: s, Folder: "inbox"})
	}
	g.Create(&model.Mail{UserID: u.ID, From: "d@x", To: u.Email, Subject: "Other", Folder: "inbox", Read: true})

	mb := &MailBox{DB: g}
	tok, _ := auth.Sign(u.ID, u.Email, 0)
	req := httptest.NewRequest("GET", "/api/mails?folder=inbox&group=thread", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	mb.List(rr, req)

	var out struct {
		Total int
		Items []model.Mail
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析失败: %v body=%s", err, rr.Body.String())
	}
	if out.Total != 2 {
		t.Fatalf("应 2 个会话，得到 %d（%s）", out.Total, rr.Body.String())
	}
	var hello *model.Mail
	for i := range out.Items {
		if out.Items[i].ThreadCount == 4 {
			hello = &out.Items[i]
		}
	}
	if hello == nil {
		t.Fatalf("未找到 4 封的会话: %s", rr.Body.String())
	}
	if len(hello.ThreadIDs) != 4 {
		t.Fatalf("会话成员应 4 个，得到 %v", hello.ThreadIDs)
	}
}
