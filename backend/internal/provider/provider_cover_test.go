package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewFactory(t *testing.T) {
	if _, err := New("aliyun", map[string]string{"access_key_id": "k", "access_key_secret": "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := New("cloudflare", map[string]string{"api_token": "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestAliTTL(t *testing.T) {
	cases := map[int]int{0: 600, 599: 600, 600: 600, 1000: 1800, 4000: 43200, 50000: 86400, 100000: 86400}
	for in, want := range cases {
		if got := aliTTL(in); got != want {
			t.Errorf("aliTTL(%d)=%d want %d", in, got, want)
		}
	}
}

func TestCFErrText(t *testing.T) {
	if got := cfErrText(nil, 500); got != "HTTP 500" {
		t.Fatalf("cfErrText=%q", got)
	}
}

func TestMatchZoneEmptyName(t *testing.T) {
	zones := []Zone{{ID: "1", Name: ""}, {ID: "2", Name: "example.com"}}
	z, ok := MatchZone(zones, "a.example.com")
	if !ok || z.Name != "example.com" {
		t.Fatalf("match=%+v ok=%v", z, ok)
	}
}

func TestAliyunDeleteMismatch(t *testing.T) {
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			io.WriteString(w, `{"DomainRecords":{"Record":[{"RecordId":"r1","RR":"x","Type":"TXT","Value":"other"}]}}`)
		case "DeleteDomainRecord":
			deleted = true
			io.WriteString(w, `{"RecordId":"r1"}`)
		}
	}))
	defer srv.Close()
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"
	if err := a.DeleteRecord(context.Background(), "example.com", "x", "TXT", "wanted"); err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("mismatched value should not delete")
	}
}

func TestEnsureFailedFind(t *testing.T) {
	// 阿里云：DescribeDomainRecords 返回 400 -> 单条 failed
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") == "DescribeDomainRecords" {
			w.WriteHeader(400)
			io.WriteString(w, `{"Code":"Err","Message":"boom"}`)
			return
		}
		io.WriteString(w, `{}`)
	}))
	defer srvA.Close()
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srvA.URL + "/"
	res, err := a.EnsureRecords(context.Background(), "example.com", []Record{{Name: "@", Type: "TXT", Value: "v"}})
	if err != nil || res[0].Action != "failed" {
		t.Fatalf("aliyun res=%+v err=%v", res, err)
	}

	// Cloudflare：dns_records 返回 success:false -> 单条 failed
	srvC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/dns_records") {
			io.WriteString(w, `{"success":false,"errors":[{"code":1,"message":"boom"}]}`)
			return
		}
		if r.Method == "GET" {
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)
			return
		}
		io.WriteString(w, `{"success":true,"result":{}}`)
	}))
	defer srvC.Close()
	c, _ := newCloudflare(map[string]string{"api_token": "t"})
	c.base = srvC.URL + "/client/v4"
	resC, err := c.EnsureRecords(context.Background(), "example.com", []Record{{Name: "@", Type: "TXT", Value: "v"}})
	if err != nil || resC[0].Action != "failed" {
		t.Fatalf("cloudflare res=%+v err=%v", resC, err)
	}
}

func TestBadBaseErrors(t *testing.T) {
	c, _ := newCloudflare(map[string]string{"api_token": "t"})
	c.base = "http://bad\x7fhost"
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("invalid base should error")
	}
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = "http://bad\x7fhost/"
	if err := a.Verify(context.Background()); err == nil {
		t.Fatal("invalid base should error")
	}
}
