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

// #9：逐会话踢出——删除 session 后该 token 立即失效。
func TestSessionRevocation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, sessions, login_events RESTART IDENTITY CASCADE")
	auth.SetSecret("unit-test-secret-0123456789")
	hash, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.DefaultCost)
	u := model.User{Email: "sess@test.local", PassHash: string(hash)}
	g.Create(&u)
	a := &Auth{DB: g}

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"email":"sess@test.local","password":"Passw0rd!"}`))
	req.RemoteAddr = "203.0.113.1:1"
	rr := httptest.NewRecorder()
	a.Login(rr, req)
	if rr.Code != 200 {
		t.Fatalf("登录应 200，得到 %d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)

	use := func() bool {
		r2 := httptest.NewRequest("GET", "/", nil)
		r2.Header.Set("Authorization", "Bearer "+out.Token)
		_, ok := uidOf(g, httptest.NewRecorder(), r2)
		return ok
	}
	if !use() {
		t.Fatal("登录后 token 应有效")
	}
	var s model.Session
	if err := g.First(&s).Error; err != nil {
		t.Fatal("应创建会话")
	}
	g.Delete(&s)
	if use() {
		t.Fatal("踢出会话后 token 应失效")
	}
}
