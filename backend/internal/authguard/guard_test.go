package authguard

import (
	"testing"
	"time"
)

func TestGuardBlockAfterMax(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	g := New(3, time.Minute)
	g.now = func() time.Time { return now }

	if g.Blocked("1.2.3.4") {
		t.Fatal("初始不应封禁")
	}
	for i := 1; i <= 3; i++ {
		if n := g.Fail("1.2.3.4"); n != i {
			t.Fatalf("第 %d 次失败计数=%d", i, n)
		}
	}
	if !g.Blocked("1.2.3.4") {
		t.Fatal("达到上限后应封禁")
	}
	if g.Blocked("5.6.7.8") {
		t.Fatal("其它 IP 不受影响")
	}

	// 窗口过期后解封并清零
	now = now.Add(time.Minute + time.Second)
	if g.Blocked("1.2.3.4") {
		t.Fatal("窗口过期应解封")
	}
	if n := g.Fail("1.2.3.4"); n != 1 {
		t.Fatalf("过期后应重新计数，得到 %d", n)
	}
}

func TestGuardReset(t *testing.T) {
	g := New(2, time.Minute)
	g.Fail("ip")
	g.Fail("ip")
	if !g.Blocked("ip") {
		t.Fatal("应封禁")
	}
	g.Reset("ip")
	if g.Blocked("ip") {
		t.Fatal("Reset 后应解封")
	}
}
