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

// 收件账户去重列表：排除主邮箱与已发送。
func TestRecipients(t *testing.T) {
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
	u := model.User{Email: "me@test.local"}
	g.Create(&u)
	g.Create(&model.Mail{UserID: u.ID, From: "a@x", To: "sales@test.local", Folder: "inbox"})
	g.Create(&model.Mail{UserID: u.ID, From: "b@x", To: "support@test.local, me@test.local", Folder: "inbox"})
	g.Create(&model.Mail{UserID: u.ID, From: u.Email, To: "d@e.f", Folder: "sent"})

	mb := &MailBox{DB: g}
	tok, _ := auth.Sign(u.ID, u.Email, 0)
	req := httptest.NewRequest("GET", "/api/mails/recipients", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	mb.Recipients(rr, req)

	var out []string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析失败: %v body=%s", err, rr.Body.String())
	}
	got := map[string]bool{}
	for _, x := range out {
		got[x] = true
	}
	if !got["sales@test.local"] || !got["support@test.local"] {
		t.Fatalf("应含 sales/support，得到 %v", out)
	}
	if got["me@test.local"] || got["d@e.f"] {
		t.Fatalf("不应含主邮箱/sent 收件人，得到 %v", out)
	}
}
