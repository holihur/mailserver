package mailqueue

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #12：每用户每日发信配额超限后拒绝入队并标记失败。需要 PostgreSQL 与 Redis。
func TestAllowSendDailyLimit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379/0"
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails RESTART IDENTITY CASCADE")
	u := model.User{Email: "lim@test.local"}
	g.Create(&u)
	m := model.Mail{UserID: u.ID, From: u.Email, To: "x@y.z", Folder: "sent", Status: "queued"}
	g.Create(&m)

	c, err := NewClient(redisURL, g, 2, 0)
	if err != nil {
		t.Skipf("Redis 不可用，跳过: %v", err)
	}
	defer c.Close()
	ctx := context.Background()
	// NewClient 不会真正连接，这里显式 Ping，避免无 Redis 环境下误判失败。
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis 不可用，跳过: %v", err)
	}
	key := fmt.Sprintf("send:day:%d:%s", u.ID, time.Now().Format("20060102"))
	c.rdb.Del(ctx, key)

	if !c.allowSend(m.ID) {
		t.Fatal("第 1 封应允许")
	}
	if !c.allowSend(m.ID) {
		t.Fatal("第 2 封应允许")
	}
	if c.allowSend(m.ID) {
		t.Fatal("第 3 封应超限被拒")
	}
	var fresh model.Mail
	g.First(&fresh, m.ID)
	if fresh.Status != "failed" {
		t.Fatalf("超限应标记 failed，得到 %q (%s)", fresh.Status, fresh.RelayErr)
	}
}

// 按用户覆盖发信上限（覆盖全局默认）。
func TestAllowSendPerUserOverride(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379/0"
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails RESTART IDENTITY CASCADE")
	u := model.User{Email: "ov@test.local", SendDailyLimit: 1}
	g.Create(&u)
	m := model.Mail{UserID: u.ID, From: u.Email, To: "x@y.z", Folder: "sent", Status: "queued"}
	g.Create(&m)

	c, err := NewClient(redisURL, g, 100, 0) // 全局每日 100，用户覆盖为 1
	if err != nil {
		t.Skipf("Redis 不可用，跳过: %v", err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis 不可用，跳过: %v", err)
	}
	c.rdb.Del(ctx, fmt.Sprintf("send:day:%d:%s", u.ID, time.Now().Format("20060102")))

	if !c.allowSend(m.ID) {
		t.Fatal("第 1 封应允许")
	}
	if c.allowSend(m.ID) {
		t.Fatal("第 2 封应被用户上限 1 拒绝")
	}
}
