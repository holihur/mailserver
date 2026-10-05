// Package push 是进程内 JMAP 状态推送中枢：邮件变更时通知该用户已建立的
// EventSource 长连接，推送 StateChange。单进程部署下无需 Redis。
package push

import (
	"strconv"
	"sync"
)

var (
	mu   sync.Mutex
	subs = map[uint]map[chan string]bool{}
	seq  uint64
)

// Notify 通知某用户：与其邮件相关的状态已变化，推送新的 state。
func Notify(uid uint) {
	mu.Lock()
	seq++
	state := strconv.FormatUint(seq, 10)
	chans := make([]chan string, 0, len(subs[uid]))
	for ch := range subs[uid] {
		chans = append(chans, ch)
	}
	mu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- state:
		default: // 客户端消费慢时丢弃，后续会以新 state 覆盖
		}
	}
}

// Subscribe 订阅某用户的状态变更，返回接收 state 的通道。
func Subscribe(uid uint) chan string {
	ch := make(chan string, 8)
	mu.Lock()
	if subs[uid] == nil {
		subs[uid] = map[chan string]bool{}
	}
	subs[uid][ch] = true
	mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅。
func Unsubscribe(uid uint, ch chan string) {
	mu.Lock()
	if subs[uid] != nil {
		delete(subs[uid], ch)
		if len(subs[uid]) == 0 {
			delete(subs, uid)
		}
	}
	mu.Unlock()
}

// Current 返回当前全局序号（作为初始 state）。
func Current() string {
	mu.Lock()
	defer mu.Unlock()
	return strconv.FormatUint(seq, 10)
}
