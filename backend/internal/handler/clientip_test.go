package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newIPReq(remote string, hdr map[string]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestClientIP(t *testing.T) {
	// 直连公网：忽略伪造的 XFF
	if got := ClientIP(newIPReq("203.0.113.9:1234", map[string]string{"X-Forwarded-For": "1.2.3.4"})); got != "203.0.113.9" {
		t.Fatalf("公网直连应忽略 XFF，得到 %s", got)
	}
	// 本机代理：信任 XFF 首个
	if got := ClientIP(newIPReq("127.0.0.1:1234", map[string]string{"X-Forwarded-For": "1.2.3.4, 10.0.0.1"})); got != "1.2.3.4" {
		t.Fatalf("本机代理应取 XFF 首个，得到 %s", got)
	}
	// 私网代理 + X-Real-IP
	if got := ClientIP(newIPReq("172.18.0.2:1234", map[string]string{"X-Real-IP": "5.6.7.8"})); got != "5.6.7.8" {
		t.Fatalf("私网代理应取 X-Real-IP，得到 %s", got)
	}
	// 无代理头：回退 RemoteAddr
	if got := ClientIP(newIPReq("127.0.0.1:1234", nil)); got != "127.0.0.1" {
		t.Fatalf("无代理头应回退，得到 %s", got)
	}
}
