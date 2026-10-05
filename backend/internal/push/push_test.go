package push

import "testing"

func TestNotifySubscribe(t *testing.T) {
	ch := Subscribe(1)
	defer Unsubscribe(1, ch)
	Notify(1)
	select {
	case state := <-ch:
		if state == "" {
			t.Fatal("state 不应为空")
		}
	default:
		t.Fatal("应收到状态变更")
	}
	// 其它用户不应收到
	other := Subscribe(2)
	defer Unsubscribe(2, other)
	Notify(1)
	select {
	case <-other:
		t.Fatal("不应收到其它用户的通知")
	default:
	}
	Unsubscribe(1, ch)
	Notify(1) // 取消订阅后不应 panic
}
