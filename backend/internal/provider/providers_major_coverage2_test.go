package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTopProvidersMissingCreds(t *testing.T) {
	cases := []struct {
		name  string
		creds map[string]string
		ctor  func(map[string]string) (Provider, error)
	}{
		{"digitalocean", map[string]string{}, func(c map[string]string) (Provider, error) { return newDigitalOcean(c) }},
		{"vultr", map[string]string{}, func(c map[string]string) (Provider, error) { return newVultr(c) }},
		{"linode", map[string]string{}, func(c map[string]string) (Provider, error) { return newLinode(c) }},
		{"godaddy-key", map[string]string{"api_key": "k"}, func(c map[string]string) (Provider, error) { return newGoDaddy(c) }},
		{"godaddy-secret", map[string]string{"api_secret": "s"}, func(c map[string]string) (Provider, error) { return newGoDaddy(c) }},
		{"tencent-id", map[string]string{"secret_id": "id"}, func(c map[string]string) (Provider, error) { return newTencent(c) }},
		{"tencent-key", map[string]string{"secret_key": "key"}, func(c map[string]string) (Provider, error) { return newTencent(c) }},
	}
	for _, c := range cases {
		if _, err := c.ctor(c.creds); err == nil {
			t.Errorf("%s: 缺少凭证应报错", c.name)
		}
	}
}

// EnsureRecords 在查找记录出错时，应把该条标记为 failed 而不是整体报错。
func TestTopProvidersEnsureFailedBranch(t *testing.T) {
	ctx := context.Background()
	rec := []Record{{Name: "mail", Type: "A", Value: "1.2.3.4", TTL: 600}}

	t.Run("digitalocean", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"message":"boom"}`)
		}))
		defer srv.Close()
		d, _ := newDigitalOcean(map[string]string{"api_token": "t"})
		d.base = srv.URL
		res, err := d.EnsureRecords(ctx, "a.com", rec)
		if err != nil || res[0].Action != "failed" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("vultr", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"error":"boom"}`)
		}))
		defer srv.Close()
		v, _ := newVultr(map[string]string{"api_token": "t"})
		v.base = srv.URL
		res, err := v.EnsureRecords(ctx, "a.com", rec)
		if err != nil || res[0].Action != "failed" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("linode", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/domains" {
				_, _ = io.WriteString(w, `{"data":[{"id":5,"domain":"a.com"}],"pages":1}`)
				return
			}
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"errors":[{"reason":"boom"}]}`)
		}))
		defer srv.Close()
		l, _ := newLinode(map[string]string{"api_token": "t"})
		l.base = srv.URL
		res, err := l.EnsureRecords(ctx, "a.com", rec)
		if err != nil || res[0].Action != "failed" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("godaddy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"code":"ERR","message":"boom"}`)
		}))
		defer srv.Close()
		g, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
		g.base = srv.URL
		res, err := g.EnsureRecords(ctx, "a.com", rec)
		if err != nil || res[0].Action != "failed" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("tencent", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"Response":{"Error":{"Code":"InternalError","Message":"boom"}}}`)
		}))
		defer srv.Close()
		tt, _ := newTencent(map[string]string{"secret_id": "id", "secret_key": "key"})
		tt.base = srv.URL
		res, err := tt.EnsureRecords(ctx, "a.com", rec)
		if err != nil || res[0].Action != "failed" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})
}

func TestTopProvidersBadJSON(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	d, _ := newDigitalOcean(map[string]string{"api_token": "t"})
	d.base = srv.URL
	if err := d.Verify(ctx); err == nil {
		t.Fatal("digitalocean bad json should error")
	}
	v, _ := newVultr(map[string]string{"api_token": "t"})
	v.base = srv.URL
	if err := v.Verify(ctx); err == nil {
		t.Fatal("vultr bad json should error")
	}
	g, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	g.base = srv.URL
	if err := g.Verify(ctx); err == nil {
		t.Fatal("godaddy bad json should error")
	}
}

func TestProviderErrorMessageWrapping(t *testing.T) {
	// 无 message 字段时，错误信息回退为原始响应体。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `plain failure`)
	}))
	defer srv.Close()
	d, _ := newDigitalOcean(map[string]string{"api_token": "t"})
	d.base = srv.URL
	if err := d.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "plain failure") {
		t.Fatalf("err=%v", err)
	}
}
