package handler

import (
	"os"
	"testing"
	"time"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

// 会话/登录历史保留策略：过期会话与 >90 天登录历史被清理。
func TestPurgeAuth(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, sessions, login_events RESTART IDENTITY CASCADE")
	u := model.User{Email: "ret@test.local"}
	g.Create(&u)
	now := time.Now()
	g.Create(&model.Session{JTI: "expired", UserID: u.ID, ExpiresAt: now.Add(-time.Hour)})
	g.Create(&model.Session{JTI: "valid", UserID: u.ID, ExpiresAt: now.Add(time.Hour)})
	g.Create(&model.LoginEvent{UserID: u.ID, Email: u.Email, CreatedAt: now.AddDate(0, 0, -100)})
	g.Create(&model.LoginEvent{UserID: u.ID, Email: u.Email, CreatedAt: now.AddDate(0, 0, -1)})

	s, e := PurgeAuth(g)
	if s != 1 || e != 1 {
		t.Fatalf("应清理 1 会话 / 1 登录历史，得到 %d/%d", s, e)
	}
	var ns, ne int64
	g.Model(&model.Session{}).Count(&ns)
	g.Model(&model.LoginEvent{}).Count(&ne)
	if ns != 1 || ne != 1 {
		t.Fatalf("应剩 1/1，得到 %d/%d", ns, ne)
	}
}
