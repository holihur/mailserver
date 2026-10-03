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

func TestRecordFQDN(t *testing.T) {
	cases := []struct{ name, want string }{
		{"@", "example.com"},
		{"", "example.com"},
		{"mail", "mail.example.com"},
		{"_dmarc", "_dmarc.example.com"},
		{"dkim._domainkey", "dkim._domainkey.example.com"},
		{"mail.example.com", "mail.example.com"},
	}
	for _, c := range cases {
		if got := recordFQDN("example.com", c.name); got != c.want {
			t.Errorf("recordFQDN(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCFTTL(t *testing.T) {
	if cfTTL(0) != 1 || cfTTL(1) != 1 || cfTTL(500) != 600 || cfTTL(999999) != 86400 {
		t.Fatalf("cfTTL mapping wrong: %d %d %d %d", cfTTL(0), cfTTL(1), cfTTL(500), cfTTL(999999))
	}
}

func TestCloudflare(t *testing.T) {
	var created []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/dns_records"):
			io.WriteString(w, `{"success":true,"result":[]}`)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/dns_records"):
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			json.Unmarshal(b, &m)
			created = append(created, m)
			io.WriteString(w, `{"success":true,"result":{"id":"new"}}`)
		case r.Method == "GET" && r.URL.Query().Get("name") == "example.com":
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/client/v4/zones"):
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}],"result_info":{"page":1,"total_pages":1}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			io.WriteString(w, `{"success":false,"errors":[{"code":1,"message":"unexpected"}]}`)
		}
	}))
	defer srv.Close()

	c, err := newCloudflare(map[string]string{"api_token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	c.base = srv.URL + "/client/v4"

	zs, err := c.ListZones(context.Background())
	if err != nil || len(zs) != 1 || zs[0].ID != "z1" {
		t.Fatalf("ListZones: %+v, %v", zs, err)
	}

	res, err := c.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "1.2.3.4", TTL: 600},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Action != "created" || res[1].Action != "created" {
		t.Fatalf("unexpected results: %+v", res)
	}
	if len(created) != 2 {
		t.Fatalf("want 2 creates, got %d", len(created))
	}
	if created[0]["name"] != "mail.example.com" || created[0]["type"] != "A" {
		t.Fatalf("bad A payload: %v", created[0])
	}
	if created[1]["name"] != "example.com" || created[1]["priority"] != float64(10) {
		t.Fatalf("bad MX payload: %v", created[1])
	}
}

func TestCloudflareGlobalKeyAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Email") != "a@b.com" || r.Header.Get("X-Auth-Key") != "gk" {
			t.Errorf("global key auth headers missing: %v", r.Header)
		}
		io.WriteString(w, `{"success":true,"result":[]}`)
	}))
	defer srv.Close()

	c, err := newCloudflare(map[string]string{"email": "a@b.com", "api_key": "gk"})
	if err != nil {
		t.Fatal(err)
	}
	c.base = srv.URL + "/client/v4"
	if err := c.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}
