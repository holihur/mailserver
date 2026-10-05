package jmap

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mailserver/internal/push"
)

// RFC 8620 §7：EventSource 建连发 ping，邮件变更推送 state。
func TestEventSourcePush(t *testing.T) {
	s, _, token, uid := setup(t)
	srv := httptest.NewServer(http.HandlerFunc(s.Handler))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/jmap/eventsource/?types=*&closeafter=no&ping=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d ct=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	br := bufio.NewReader(resp.Body)
	readEvent := func() (string, string) {
		var ev, data string
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				t.Fatalf("读取事件失败: %v", err)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				return ev, data
			}
			if strings.HasPrefix(line, "event:") {
				ev = strings.TrimSpace(line[len("event:"):])
			}
			if strings.HasPrefix(line, "data:") {
				data = strings.TrimSpace(line[len("data:"):])
			}
		}
	}

	if ev, _ := readEvent(); ev != "ping" {
		t.Fatalf("建连应先发 ping，得到 %q", ev)
	}
	push.Notify(uid)
	ev, data := readEvent()
	if ev != "state" {
		t.Fatalf("应推送 state，得到 %q", ev)
	}
	if !strings.Contains(data, "StateChange") || !strings.Contains(data, acctID(uid)) || !strings.Contains(data, "Email") {
		t.Fatalf("state 数据不正确: %s", data)
	}
}
