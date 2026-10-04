package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// #19：/metrics 需鉴权：配 token 用 Bearer；否则仅本机。
func TestMetricsAuth(t *testing.T) {
	m := &MetricsBox{Token: "secret", Version: "v1", Commit: "abc"}

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()
	m.Serve(rr, req)
	if rr.Code != 403 {
		t.Fatalf("无鉴权应 403，得到 %d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	m.Serve(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "sweetcorn_build_info") {
		t.Fatalf("正确 token 应 200 且含指标，得到 %d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rr = httptest.NewRecorder()
	m.Serve(rr, req)
	if rr.Code != 403 {
		t.Fatalf("错误 token 应 403，得到 %d", rr.Code)
	}

	// 未配 token：仅本机可访问
	m2 := &MetricsBox{}
	req = httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rr = httptest.NewRecorder()
	m2.Serve(rr, req)
	if rr.Code != 200 {
		t.Fatalf("本机应 200，得到 %d", rr.Code)
	}
	req = httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "203.0.113.5:5555"
	rr = httptest.NewRecorder()
	m2.Serve(rr, req)
	if rr.Code != 403 {
		t.Fatalf("外网应 403，得到 %d", rr.Code)
	}
}
