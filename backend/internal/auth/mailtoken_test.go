package auth

import (
	"os"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

func TestNewMailToken(t *testing.T) {
	p1, pre1, err := NewMailToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) < 20 || p1[:len(MailTokenPrefix)] != MailTokenPrefix {
		t.Fatalf("令牌格式不对: %q", p1)
	}
	if pre1 != p1[:12] {
		t.Fatalf("prefix 应为前 12 位，得到 %q", pre1)
	}
	p2, _, _ := NewMailToken()
	if p1 == p2 {
		t.Fatal("两次生成的令牌不应相同")
	}
	if HashMailToken(p1) == HashMailToken(p2) {
		t.Fatal("不同令牌的摘要不应相同")
	}
	if HashMailToken(p1) != HashMailToken("  "+p1+"  ") {
		t.Fatal("摘要计算应忽略首尾空白")
	}
	if len(HashMailToken(p1)) != 64 {
		t.Fatal("摘要应为 64 位十六进制（sha256）")
	}
}

// TestAuthenticateMail 需要 TEST_DATABASE_URL，否则跳过（CI 有 postgres 服务）。
func TestAuthenticateMail(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.Exec("TRUNCATE mail_tokens, users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}

	plain, prefix, _ := NewMailToken()
	u := model.User{Email: "pat@test.local", Name: "pat"}
	if err := g.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tok := model.MailToken{UserID: u.ID, Name: "phone", Prefix: prefix, Hash: HashMailToken(plain)}
	if err := g.Create(&tok).Error; err != nil {
		t.Fatal(err)
	}

	// 正确令牌
	got, err := AuthenticateMail(g, "pat@test.local", plain)
	if err != nil || got.ID != u.ID {
		t.Fatalf("有效令牌应通过: user=%v err=%v", got, err)
	}
	// 邮箱大小写不敏感
	if _, err := AuthenticateMail(g, "PAT@test.local", plain); err != nil {
		t.Fatalf("邮箱应大小写不敏感: %v", err)
	}
	// last_used 被更新
	var fresh model.MailToken
	g.First(&fresh, tok.ID)
	if fresh.LastUsed == nil {
		t.Fatal("last_used 应被更新")
	}
	// 网页登录密码（非 mst_ 前缀）必须被拒绝
	if _, err := AuthenticateMail(g, "pat@test.local", "my-login-password"); err == nil {
		t.Fatal("登录密码不应被接受")
	}
	// 伪造令牌
	if _, err := AuthenticateMail(g, "pat@test.local", "mst_bogus"); err == nil {
		t.Fatal("无效令牌应失败")
	}
	// 令牌与邮箱不匹配
	if _, err := AuthenticateMail(g, "other@test.local", plain); err == nil {
		t.Fatal("邮箱不匹配应失败")
	}
	// 账号禁用
	if err := g.Model(&u).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AuthenticateMail(g, "pat@test.local", plain); err == nil {
		t.Fatal("禁用账号应失败")
	}
}
