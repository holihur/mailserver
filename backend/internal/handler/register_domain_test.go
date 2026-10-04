package handler

import (
	"os"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

func TestIsHostedEmailDomain(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE domains, dns_records RESTART IDENTITY CASCADE")
	g.Create(&model.Domain{Name: "hosted.com"})
	a := &Auth{DB: g}

	if !a.isHostedEmailDomain("hosted.com") {
		t.Fatal("托管域名应允许")
	}
	if !a.isHostedEmailDomain("HOSTED.COM") {
		t.Fatal("域名匹配应大小写不敏感")
	}
	if a.isHostedEmailDomain("mail.hosted.com") {
		t.Fatal("只认 domains 表，子域不应放行")
	}
	if a.isHostedEmailDomain("gmail.com") {
		t.Fatal("未托管域名应拒绝")
	}
	if a.isHostedEmailDomain("") {
		t.Fatal("空域名应拒绝")
	}
}
