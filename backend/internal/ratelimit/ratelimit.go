// Package ratelimit 使用 Redis 做登录/注册限流（不提供内存回退；Redis 故障时按拒绝处理）。
package ratelimit

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb *redis.Client
}

// New 连接 Redis；url 为空或连接失败直接返回错误（调用方应据此拒绝启动）。
func New(url string) (*Limiter, error) {
	if url == "" {
		return nil, errors.New("未配置 REDIS_URL（限流使用 Redis）")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Limiter{rdb: rdb}, nil
}

// Allow 在 window 内最多允许 max 次。Redis 出错时返回 false（fail-closed）。
func (l *Limiter) Allow(key string, max int, window time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	full := "rl:" + key
	n, err := l.rdb.Incr(ctx, full).Result()
	if err != nil {
		log.Printf("ratelimit: redis 错误，拒绝本次请求: %v", err)
		return false
	}
	if n == 1 {
		_ = l.rdb.Expire(ctx, full, window).Err()
	}
	return n <= int64(max)
}
