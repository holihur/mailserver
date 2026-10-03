package db

import (
	"os"
	"testing"

	"mailserver/internal/model"
)

// TestPostgresIntegration 仅在设置了 TEST_DATABASE_URL 时运行（CI 用 postgres 服务）。
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 PostgreSQL 集成测试")
	}
	g, err := Open("postgres", "", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := g.Exec("TRUNCATE users, mails, domains, dns_records, dns_providers, acme_configs, settings RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}

	u := model.User{Email: "pg@test.local", Name: "pg"}
	if err := g.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := g.Create(&model.Mail{UserID: u.ID, From: "a@b.c", To: "x@y.z", Subject: "hi", Folder: "inbox"}).Error; err != nil {
		t.Fatalf("create mail: %v", err)
	}

	// 保留字列名 read 通过 map 条件（跨库可移植）
	var unread int64
	g.Model(&model.Mail{}).Where("user_id = ?", u.ID).Where(map[string]any{"read": false}).Count(&unread)
	if unread != 1 {
		t.Fatalf("want 1 unread, got %d", unread)
	}

	// 双引号标识符的搜索查询在 PG 下必须可用
	var found int64
	if err := g.Model(&model.Mail{}).
		Where("subject LIKE ? OR \"from\" LIKE ? OR \"to\" LIKE ?", "%hi%", "%a%", "%b%").
		Count(&found).Error; err != nil {
		t.Fatalf("search query: %v", err)
	}
	if found != 1 {
		t.Fatalf("want 1 match, got %d", found)
	}

	// 设置表 upsert
	if err := g.Create(&model.Setting{Key: "k", Value: "v"}).Error; err != nil {
		t.Fatal(err)
	}
}
