package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewTopProviders(t *testing.T) {
	cases := map[string]map[string]string{
		"tencent":      {"secret_id": "id", "secret_key": "key"},
		"godaddy":      {"api_key": "k", "api_secret": "s"},
		"digitalocean": {"api_token": "t"},
		"vultr":        {"api_token": "t"},
		"linode":       {"api_token": "t"},
	}
	for kind, creds := range cases {
		if _, err := New(kind, creds); err != nil {
			t.Errorf("New(%q): %v", kind, err)
		}
	}
	if _, err := New("digitalocean", nil); err == nil {
		t.Error("missing token should error")
	}
}

func TestDigitalOcean(t *testing.T) {
	var put, post int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/domains") && !strings.Contains(r.URL.Path, "/records"):
			io.WriteString(w, `{"domains":[{"name":"example.com"}]}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			io.WriteString(w, `{"domain_records":[{"id":1,"type":"A","name":"mail","data":"1.1.1.1","ttl":1800}]}`)
		case r.Method == "PUT":
			put++
			io.WriteString(w, `{}`)
		case r.Method == "POST":
			post++
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	d, err := newDigitalOcean(map[string]string{"api_token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	d.base = srv.URL

	zs, err := d.ListZones(context.Background())
	if err != nil || len(zs) != 1 || zs[0].Name != "example.com" {
		t.Fatalf("zones=%+v err=%v", zs, err)
	}
	res, err := d.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 1800},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || res[1].Action != "created" {
		t.Fatalf("results=%+v", res)
	}
	if put != 1 || post != 1 {
		t.Fatalf("put=%d post=%d", put, post)
	}
}

func TestVultr(t *testing.T) {
	var patched, created bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/domains":
			io.WriteString(w, `{"domains":[{"domain":"example.com"}],"meta":{"links":{"next":""}}}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			io.WriteString(w, `{"records":[{"id":"r1","type":"A","name":"mail","data":"1.1.1.1","ttl":300,"priority":0}],"meta":{"links":{"next":""}}}`)
		case r.Method == "PATCH":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"data":"2.2.2.2"`) {
				t.Errorf("bad patch body: %s", body)
			}
			patched = true
			io.WriteString(w, `{}`)
		case r.Method == "POST":
			var m map[string]any
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &m)
			if m["name"] != "" {
				t.Errorf("vultr root name should be empty, got %v", m["name"])
			}
			created = true
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	v, err := newVultr(map[string]string{"api_token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	v.base = srv.URL

	res, err := v.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 300},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || res[1].Action != "created" || !patched || !created {
		t.Fatalf("results=%+v patched=%v created=%v", res, patched, created)
	}
}

func TestLinode(t *testing.T) {
	var put, post int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/domains":
			io.WriteString(w, `{"data":[{"id":5,"domain":"example.com"}],"pages":1}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			io.WriteString(w, `{"data":[{"id":1,"type":"A","name":"mail","target":"1.1.1.1","ttl_sec":300,"priority":0}],"pages":1}`)
		case r.Method == "PUT":
			put++
			io.WriteString(w, `{}`)
		case r.Method == "POST":
			var m map[string]any
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &m)
			if m["name"] != "" {
				t.Errorf("linode root name should be empty, got %v", m["name"])
			}
			post++
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	l, err := newLinode(map[string]string{"api_token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	l.base = srv.URL

	res, err := l.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 300},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || res[1].Action != "created" || put != 1 || post != 1 {
		t.Fatalf("results=%+v put=%d post=%d", res, put, post)
	}
}

func TestGoDaddy(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "sso-key k:s" {
			t.Errorf("bad auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/domains":
			io.WriteString(w, `[{"domainId":"1","domain":"example.com","status":"ACTIVE"}]`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records/A/mail"):
			io.WriteString(w, `[{"data":"1.1.1.1","name":"mail","ttl":600,"type":"A"}]`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records/MX/"):
			io.WriteString(w, `[]`)
		case r.Method == "PUT":
			puts++
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	g, err := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	if err != nil {
		t.Fatal(err)
	}
	g.base = srv.URL

	zs, err := g.ListZones(context.Background())
	if err != nil || len(zs) != 1 || zs[0].Name != "example.com" {
		t.Fatalf("zones=%+v err=%v", zs, err)
	}
	res, err := g.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 600},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || res[1].Action != "created" || puts != 2 {
		t.Fatalf("results=%+v puts=%d", res, puts)
	}
}

func TestTopProviderTTLs(t *testing.T) {
	if doTTL(10) != 30 || doTTL(100000) != 86400 {
		t.Fatal("doTTL")
	}
	if vultrTTL(10) != 300 || vultrTTL(100000) != 86400 {
		t.Fatal("vultrTTL")
	}
	if linodeTTL(10) != 300 || linodeTTL(100000) != 86400 {
		t.Fatal("linodeTTL")
	}
	if godaddyTTL(10) != 600 || godaddyTTL(100000) != 86400 {
		t.Fatal("godaddyTTL")
	}
}
