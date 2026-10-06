package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupAndNormalize(t *testing.T) {
	if _, ok := Lookup(" Openai "); !ok {
		t.Fatal("Lookup 应大小写/空白不敏感")
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("未知服务商不应命中")
	}
	if got := NormalizeBaseURL("openai", ""); got != "https://api.openai.com/v1" {
		t.Fatalf("空 BaseURL 应回退目录默认值，得到 %q", got)
	}
	if got := NormalizeBaseURL("custom", " https://x.test/v1/ "); got != "https://x.test/v1" {
		t.Fatalf("显式 BaseURL 应去尾斜杠，得到 %q", got)
	}
}

func TestCatalogIsCopy(t *testing.T) {
	c := Catalog()
	if len(c) == 0 {
		t.Fatal("目录不应为空")
	}
	c[0].Label = "mutated"
	if Catalog()[0].Label == "mutated" {
		t.Fatal("Catalog 应返回副本，外部修改不应影响内部")
	}
}

func TestTestSendsBearerAndSucceeds(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := Config{Provider: "openai", BaseURL: srv.URL + "/v1", APIKey: "sk-test"}
	if err := Test(context.Background(), cfg); err != nil {
		t.Fatalf("有效端点应通过：%v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("应发送 Bearer Key，得到 %q", gotAuth)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("应请求 /models，得到 %q", gotPath)
	}
}

func TestTestAnthropicHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := Test(context.Background(), Config{Provider: "anthropic", BaseURL: srv.URL, APIKey: "ak"}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "ak" {
		t.Fatalf("Anthropic 应使用 x-api-key，得到 %q", gotKey)
	}
}

func TestTestFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if err := Test(context.Background(), Config{Provider: "openai", BaseURL: srv.URL, APIKey: "x"}); err == nil {
		t.Fatal("401 应报错")
	}
}

func TestTestRejectsBadBaseURL(t *testing.T) {
	if err := Test(context.Background(), Config{Provider: "custom", BaseURL: "ftp://x"}); err == nil {
		t.Fatal("非 http(s) 应报错")
	}
	if err := Test(context.Background(), Config{Provider: "custom"}); err == nil {
		t.Fatal("缺少 BaseURL 应报错")
	}
}
