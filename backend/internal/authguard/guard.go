// Package authguard 提供不依赖 Redis 的进程内认证失败限流，用于协议层（IMAP/POP3/
// SMTP 提交/ManageSieve）防止撞库与连接耗尽。按 IP 计数，固定窗口。
package authguard

import (
	"sync"
	"time"
)

const (
	// MaxFails 窗口内允许的最大失败次数。
	MaxFails = 30
	// Window 计数窗口。
	Window = 10 * time.Minute
)

type Guard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	max     int
	window  time.Duration
	now     func() time.Time
}

type bucket struct {
	count int
	reset time.Time
}

func New(max int, window time.Duration) *Guard {
	return &Guard{buckets: map[string]*bucket{}, max: max, window: window, now: time.Now}
}

// Blocked 判断 key（通常是来源 IP）是否已超过失败上限。
func (g *Guard) Blocked(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	b := g.buckets[key]
	if b == nil || g.now().After(b.reset) {
		return false
	}
	return b.count >= g.max
}

// Fail 记录一次失败，返回当前窗口内的失败次数。
func (g *Guard) Fail(key string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	b := g.buckets[key]
	if b == nil || now.After(b.reset) {
		b = &bucket{reset: now.Add(g.window)}
		g.buckets[key] = b
	}
	b.count++
	return b.count
}

// Reset 在认证成功后清除该 key 的失败计数。
func (g *Guard) Reset(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.buckets, key)
}

// Default 全局默认限流器：10 分钟 30 次失败。
var Default = New(MaxFails, Window)
