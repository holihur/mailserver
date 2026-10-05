// Package mailqueue 用 asynq（Redis 后端）异步处理发信。
// 发送邮件入队后由 worker 调用 queue.Deliver 投递，并维护 queued/sending/sent/failed 状态。
// 另有周期性 sweep 任务兜底：把库里仍未发出的 sent 邮件入队（转发/别名/提交产生的邮件）。
package mailqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"mailserver/internal/contacts"
	"mailserver/internal/model"
	"mailserver/internal/queue"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/schedule"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
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
type Client struct {
	c      *asynq.Client
	rdb    *redis.Client
	db     *gorm.DB
	daily  int
	perMin int
}

func NewClient(redisURL string, db *gorm.DB, daily, perMin int) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	ropt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &Client{c: asynq.NewClient(opt), rdb: redis.NewClient(ropt), db: db, daily: daily, perMin: perMin}, nil
}

func (c *Client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	if c.rdb != nil {
		_ = c.rdb.Close()
	}
	return c.c.Close()
}

// allowSend 每用户每日/每分钟发信限流（Redis 计数）；超限则标记邮件失败并拒绝入队。
// Redis 出错时放行（不阻断正常发信）。
func (c *Client) allowSend(mailID uint) bool {
	if c.db == nil || c.rdb == nil {
		return true
	}
	var m model.Mail
	if err := c.db.Select("user_id").First(&m, mailID).Error; err != nil || m.UserID == 0 {
		return true
	}
	// 优先使用用户单独配置的上限，否则用全局默认
	daily, perMin := c.daily, c.perMin
	var u model.User
	if err := c.db.Select("created_at", "send_daily_limit", "send_per_minute").First(&u, m.UserID).Error; err == nil {
		if u.SendDailyLimit > 0 {
			daily = u.SendDailyLimit
		}
		if u.SendPerMinute > 0 {
			perMin = u.SendPerMinute
		}
		// graduated trust：未单独设置每日上限的新账号前 24h 更严（≤ 50 封/日）
		if u.SendDailyLimit == 0 && daily > 50 && time.Since(u.CreatedAt) < 24*time.Hour {
			daily = 50
		}
	}
	if daily <= 0 && perMin <= 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	block := func(msg string) bool {
		c.db.Model(&model.Mail{}).Where("id = ?", mailID).Updates(map[string]any{"status": "failed", "relay_err": msg})
		log.Printf("[ALERT] send limit hit user=%d mail=%d: %s", m.UserID, mailID, msg)
		return false
	}
	if daily > 0 {
		k := fmt.Sprintf("send:day:%d:%s", m.UserID, time.Now().Format("20060102"))
		if n, err := c.rdb.Incr(ctx, k).Result(); err == nil {
			if n == 1 {
				_ = c.rdb.Expire(ctx, k, 24*time.Hour).Err()
			}
			if int(n) > daily {
				return block(fmt.Sprintf("超出每日发信配额（%d）", daily))
			}
		}
	}
	if perMin > 0 {
		k := fmt.Sprintf("send:min:%d:%d", m.UserID, time.Now().Unix()/60)
		if n, err := c.rdb.Incr(ctx, k).Result(); err == nil {
			if n == 1 {
				_ = c.rdb.Expire(ctx, k, 2*time.Minute).Err()
			}
			if int(n) > perMin {
				return block(fmt.Sprintf("超出每分钟发信上限（%d）", perMin))
			}
		}
	}
	return true
}

// AllowVacation 判断该（用户, 发件人）在 days 天内是否未被自动回复过（去重）。
func (c *Client) AllowVacation(uid uint, from string, days int) bool {
	if c == nil || c.rdb == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	k := fmt.Sprintf("vac:%d:%s", uid, strings.ToLower(strings.TrimSpace(from)))
	n, err := c.rdb.Incr(ctx, k).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = c.rdb.Expire(ctx, k, time.Duration(days)*24*time.Hour).Err()
	}
	return n == 1
}

// EnqueueSend 把待发邮件入队（TaskID 去重，避免重复投递）。
func (c *Client) EnqueueSend(mailID uint) error {
	if c == nil || c.c == nil {
		return nil
	}
	if !c.allowSend(mailID) {
		return fmt.Errorf("send limit exceeded")
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
	client, err := NewClient(redisURL, db, 0, 0) // 内部重试/兜底不再二次限流
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
	db      *gorm.DB
	rt      *runtimecfg.Store
	client  *Client
	sweepMu sync.Mutex
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
	// 自动把收件人（To/Cc/Bcc）收录进通讯录（系统转发的 UserID=0 会跳过）
	contacts.Collect(h.db, m.UserID, m.From, m.To, m.Cc, m.Bcc)
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
	h.sweepMu.Lock()
	defer h.sweepMu.Unlock()
	var mails []model.Mail
	h.db.Where("folder = ? AND relayed = ? AND attempts < ?", "sent", false, queue.MaxAttempts).
		Order("id").Limit(100).Find(&mails)
	for _, m := range mails {
		_ = h.client.EnqueueSend(m.ID)
	}
	h.processScheduled()
	return nil
}

// processScheduled 把到期的定时 / 周期邮件生成一封 sent 邮件并发出；重复任务计算下次时间。
func (h *handler) processScheduled() {
	var list []model.ScheduledMail
	h.db.Where("enabled = ? AND send_at <= ?", true, time.Now()).Order("send_at").Limit(100).Find(&list)
	for _, sm := range list {
		m := model.Mail{UserID: sm.UserID, From: sm.From, To: sm.To, Cc: sm.Cc, Bcc: sm.Bcc,
			Subject: sm.Subject, Body: sm.Body, Attachments: sm.Attachments, ReceiptTo: sm.ReceiptTo,
			Folder: "sent", Read: true, Status: "queued"}
		if err := h.db.Create(&m).Error; err != nil {
			h.db.Model(&model.ScheduledMail{}).Where("id = ?", sm.ID).Update("last_error", err.Error())
			continue
		}
		_ = h.client.EnqueueSend(m.ID)
		now := time.Now()
		if sm.Repeat == "" {
			h.db.Model(&model.ScheduledMail{}).Where("id = ?", sm.ID).
				Updates(map[string]any{"enabled": false, "last_sent": now, "last_error": ""})
		} else {
			next := schedule.NextRun(sm.SendAt, sm.Repeat, now)
			h.db.Model(&model.ScheduledMail{}).Where("id = ?", sm.ID).
				Updates(map[string]any{"send_at": next, "last_sent": now, "last_error": ""})
		}
	}
}
