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

	"golang.org/x/crypto/bcrypt"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		pw, email string
		ok        bool
	}{
		{"short", "a@b.c", false},                     // 太短
		{"password", "a@b.c", false},                  // 常见弱密码
		{"12345678", "a@b.c", false},                  // 常见弱密码
		{"longlocalname", "longlocalname@b.c", false}, // 等于账号名
		{"Str0ng-Pass", "alice@b.c", true},            // 合法
	}
	for _, c := range cases {
		err := validatePassword(c.pw, c.email)
		if c.ok && err != nil {
			t.Errorf("validatePassword(%q) 应通过，得到 %v", c.pw, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validatePassword(%q) 应拒绝", c.pw)
		}
	}
}

func TestChangePasswordRevokesTokens(t *testing.T) {
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

	hash, _ := bcrypt.GenerateFromPassword([]byte("OldPass123"), bcrypt.DefaultCost)
	u := model.User{Email: "cp@test.local", PassHash: string(hash)}
	if err := g.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tok0, _ := auth.Sign(u.ID, u.Email, 0)
	a := &Auth{DB: g}

	post := func(oldPw, newPw string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"old": oldPw, "new": newPw})
		req := httptest.NewRequest("POST", "/api/me/password", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+tok0)
		rr := httptest.NewRecorder()
		a.ChangePassword(rr, req)
		return rr
	}

	if rr := post("wrong", "NewPass123"); rr.Code != 400 {
		t.Fatalf("原密码错误应 400，实际 %d", rr.Code)
	}
	if rr := post("OldPass123", "short"); rr.Code != 400 {
		t.Fatalf("弱新密码应 400，实际 %d", rr.Code)
	}

	rr := post("OldPass123", "NewPass123")
	if rr.Code != 200 {
		t.Fatalf("改密应 200，实际 %d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Token == "" {
		t.Fatal("应返回新令牌")
	}

	// 旧令牌因 token_version 不符被拒
	reqOld := httptest.NewRequest("GET", "/", nil)
	reqOld.Header.Set("Authorization", "Bearer "+tok0)
	if _, ok := uidOf(g, httptest.NewRecorder(), reqOld); ok {
		t.Fatal("旧令牌应被吊销")
	}
	// 新令牌可用
	reqNew := httptest.NewRequest("GET", "/", nil)
	reqNew.Header.Set("Authorization", "Bearer "+out.Token)
	if _, ok := uidOf(g, httptest.NewRecorder(), reqNew); !ok {
		t.Fatal("新令牌应可用")
	}
}
