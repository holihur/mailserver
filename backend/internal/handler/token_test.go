package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #18：新建 PAT 默认最小权限（imap,smtp），显式全量才存全量；不再用空串代表全选。
func TestTokenCreateDefaultScopes(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mail_tokens RESTART IDENTITY CASCADE")
	auth.SetSecret("unit-test-secret-0123456789")
	u := model.User{Email: "tok@test.local"}
	g.Create(&u)
	tok, _ := auth.Sign(u.ID, u.Email, 0)
	tb := &TokenBox{DB: g}

	create := func(body string) string {
		req := httptest.NewRequest("POST", "/api/tokens", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		tb.List(rr, req)
		if rr.Code != 201 {
			t.Fatalf("创建应 201，得到 %d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Scopes string `json:"scopes"`
		}
		json.Unmarshal(rr.Body.Bytes(), &out)
		return out.Scopes
	}

	if got := create(`{"name":"a"}`); got != "imap,smtp" {
		t.Fatalf("空 scopes 默认应为 imap,smtp，得到 %q", got)
	}
	if got := create(`{"name":"b","scopes":"imap,pop3,smtp,jmap,sieve,mcp"}`); got != "imap,pop3,smtp,jmap,sieve,mcp" {
		t.Fatalf("显式全量应保留，得到 %q", got)
	}
}
