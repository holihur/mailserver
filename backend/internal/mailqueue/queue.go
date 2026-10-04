// Package mailqueue 用 asynq（Redis 后端）异步处理发信。
// 发送邮件入队后由 worker 调用 queue.Deliver 投递，并维护 queued/sending/sent/failed 状态。
// 另有周期性 sweep 任务兜底：把库里仍未发出的 sent 邮件入队（转发/别名/提交产生的邮件）。
package mailqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/queue"
	"mailserver/internal/runtimecfg"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const (
	TypeSend  = "mail:send"
	TypeSweep = "mail:sweep"
	// SendDelay 延迟发送窗口：给用户留出「撤销发送」的时间。
	SendDelay = 8 * time.Second
)

type sendPayload struct {
	ID uint `json:"id"`
}

// Client 用于把发信任务入队。
type Client struct{ c *asynq.Client }

func NewClient(redisURL string) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return &Client{c: asynq.NewClient(opt)}, nil
}

func (c *Client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close()
}

// EnqueueSend 把待发邮件入队（TaskID 去重，避免重复投递）。
func (c *Client) EnqueueSend(mailID uint) error {
	if c == nil || c.c == nil {
		return nil
	}
	payload, _ := json.Marshal(sendPayload{ID: mailID})
	_, err := c.c.Enqueue(asynq.NewTask(TypeSend, payload),
		asynq.TaskID(fmt.Sprintf("mail:%d", mailID)),
		asynq.ProcessIn(SendDelay),
		asynq.MaxRetry(queue.MaxAttempts))
	return err
}

// Start 启动 asynq worker 与定时 sweep。
func Start(redisURL string, db *gorm.DB, rt *runtimecfg.Store) error {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return err
	}
	client, err := NewClient(redisURL)
	if err != nil {
		return err
	}
	h := &handler{db: db, rt: rt, client: client}

	srv := asynq.NewServer(opt, asynq.Config{Concurrency: 5, Queues: map[string]int{"default": 1}})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeSend, h.send)
	mux.HandleFunc(TypeSweep, h.sweep)
	go func() {
		if err := srv.Run(mux); err != nil {
			log.Println("asynq server:", err)
		}
	}()

	scheduler := asynq.NewScheduler(opt, nil)
	if _, err := scheduler.Register("@every 30s", asynq.NewTask(TypeSweep, nil)); err != nil {
		return err
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			log.Println("asynq scheduler:", err)
		}
	}()
	log.Println("asynq mail queue started")
	return nil
}

type handler struct {
	db     *gorm.DB
	rt     *runtimecfg.Store
	client *Client
}

func (h *handler) send(ctx context.Context, t *asynq.Task) error {
	var p sendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return nil // 坏任务不再重试
	}
	var m model.Mail
	if err := h.db.First(&m, p.ID).Error; err != nil {
		return nil // 邮件已删除
	}
	if m.Relayed || m.Attempts >= queue.MaxAttempts {
		return nil
	}
	queue.Deliver(h.db, h.rt.Relay(), h.rt.Signer(), queue.LoadRoutes(h.db), &m)

	// 未成功则返回错误触发 asynq 重试（已到上限则不再重试）
	var fresh model.Mail
	h.db.First(&fresh, p.ID)
	if !fresh.Relayed && fresh.Attempts < queue.MaxAttempts {
		return fmt.Errorf("send failed: %s", fresh.RelayErr)
	}
	return nil
}

func (h *handler) sweep(ctx context.Context, t *asynq.Task) error {
	var mails []model.Mail
	h.db.Where("folder = ? AND relayed = ? AND attempts < ?", "sent", false, queue.MaxAttempts).
		Order("id").Limit(100).Find(&mails)
	for _, m := range mails {
		_ = h.client.EnqueueSend(m.ID)
	}
	return nil
}
