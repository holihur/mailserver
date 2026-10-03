package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormHost(t *testing.T) {
	if normHost("Mail.Example.COM.") != "mail.example.com" {
		t.Fatal("normHost")
	}
	if normHost("") != "" {
		t.Fatal("empty")
	}
}

func TestMatchZone(t *testing.T) {
	zones := []Zone{{ID: "1", Name: "example.com"}, {ID: "2", Name: "sub.example.com"}, {ID: "3", Name: "other.com"}}
	z, ok := MatchZone(zones, "mail.example.com")
	if !ok || z.Name != "example.com" {
		t.Fatalf("match=%+v ok=%v", z, ok)
	}
	z, ok = MatchZone(zones, "a.sub.example.com")
	if !ok || z.Name != "sub.example.com" {
		t.Fatalf("longest match expected, got %+v", z)
	}
	if _, ok := MatchZone(zones, "nope.org"); ok {
		t.Fatal("should not match")
	}
	if _, ok := MatchZone(nil, "example.com"); ok {
		t.Fatal("empty zones")
	}
}

func TestRelativeName(t *testing.T) {
	cases := []struct{ zone, fqdn, want string }{
		{"example.com", "example.com", "@"},
		{"example.com", "_acme-challenge.example.com", "_acme-challenge"},
		{"example.com", "_acme-challenge.mail.example.com", "_acme-challenge.mail"},
		{"example.com", "unrelated.org", "unrelated.org"},
	}
	for _, c := range cases {
		if got := RelativeName(c.zone, c.fqdn); got != c.want {
			t.Errorf("RelativeName(%q,%q)=%q want %q", c.zone, c.fqdn, got, c.want)
		}
	}
}

func TestNewUnsupported(t *testing.T) {
	if _, err := New("route53", nil); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestDecodeCreds(t *testing.T) {
	m, err := DecodeCreds(nil)
	if err != nil || len(m) != 0 {
		t.Fatalf("empty: %v %v", m, err)
	}
	m, err = DecodeCreds([]byte(`{"api_token":"x"}`))
	if err != nil || m["api_token"] != "x" {
		t.Fatalf("decode: %v %v", m, err)
	}
	if _, err := DecodeCreds([]byte("not json")); err == nil {
		t.Fatal("bad json should error")
	}
}

func TestCloudflareDeleteRecord(t *testing.T) {
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Query().Get("name") == "example.com":
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/dns_records"):
			io.WriteString(w, `{"success":true,"result":[{"id":"r1","type":"TXT","name":"_acme-challenge.example.com","content":"v"}]}`)
		case r.Method == "DELETE":
			deleted = true
			io.WriteString(w, `{"success":true,"result":{"id":"r1"}}`)
		default:
			io.WriteString(w, `{"success":false,"errors":[{"code":1,"message":"unexpected"}]}`)
		}
	}))
	defer srv.Close()

	c, _ := newCloudflare(map[string]string{"api_token": "tok"})
	c.base = srv.URL + "/client/v4"
	if err := c.DeleteRecord(context.Background(), "example.com", "_acme-challenge", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected DELETE call")
	}
}

func TestCloudflareErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("name") != "" {
			io.WriteString(w, `{"success":true,"result":[]}`) // 域名不存在
			return
		}
		io.WriteString(w, `{"success":false,"errors":[{"code":1000,"message":"bad token"}]}`)
	}))
	defer srv.Close()

	c, _ := newCloudflare(map[string]string{"api_token": "tok"})
	c.base = srv.URL + "/client/v4"
	if err := c.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("verify error=%v", err)
	}
	if _, err := c.EnsureRecords(context.Background(), "example.com", []Record{{Name: "@", Type: "MX", Value: "m"}}); err == nil {
		t.Fatal("expected zone-not-found error")
	}
	if _, err := newCloudflare(map[string]string{}); err == nil {
		t.Fatal("missing creds should error")
	}
}

func TestAliyunDeleteRecord(t *testing.T) {
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			io.WriteString(w, `{"DomainRecords":{"Record":[{"RecordId":"r1","RR":"_acme-challenge","Type":"TXT","Value":"v"}]}}`)
		case "DeleteDomainRecord":
			deleted = true
			io.WriteString(w, `{"RecordId":"r1"}`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"
	if err := a.DeleteRecord(context.Background(), "example.com", "_acme-challenge", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected DeleteDomainRecord")
	}
	// 不存在时不报错
	if err := a.DeleteRecord(context.Background(), "example.com", "nope", "TXT", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAliyunHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"Code":"InvalidAccessKeyId.NotFound","Message":"bad key"}`)
	}))
	defer srv.Close()
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"
	if err := a.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("verify error=%v", err)
	}
	if _, err := newAliyun(map[string]string{}); err == nil {
		t.Fatal("missing creds should error")
	}
	_ = json.Marshal
}
