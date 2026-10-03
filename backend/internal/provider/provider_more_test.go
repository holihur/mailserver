package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareEnsureUpdateAndUnchanged(t *testing.T) {
	var put int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/dns_records"):
			typ := r.URL.Query().Get("type")
			if typ == "A" {
				io.WriteString(w, `{"success":true,"result":[{"id":"r1","type":"A","name":"mail.example.com","content":"1.1.1.1","ttl":600}]}`)
			} else {
				io.WriteString(w, `{"success":true,"result":[{"id":"r2","type":"MX","name":"example.com","content":"mail.example.com","priority":10,"ttl":3600}]}`)
			}
		case r.Method == "GET" && r.URL.Query().Get("name") == "example.com" && !strings.Contains(r.URL.Path, "/dns_records"):
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)
		case r.Method == "PUT":
			put++
			io.WriteString(w, `{"success":true,"result":{"id":"r1"}}`)
		default:
			io.WriteString(w, `{"success":true,"result":{}}`)
		}
	}))
	defer srv.Close()

	c, _ := newCloudflare(map[string]string{"api_token": "tok"})
	c.base = srv.URL + "/client/v4"
	res, err := c.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 600},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || res[1].Action != "unchanged" {
		t.Fatalf("results=%+v", res)
	}
	if put != 1 {
		t.Fatalf("want 1 PUT, got %d", put)
	}
}

func TestCloudflareListZonesPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if page == "1" {
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"a.com"}],"result_info":{"page":1,"total_pages":2}}`)
		} else {
			io.WriteString(w, `{"success":true,"result":[{"id":"z2","name":"b.com"}],"result_info":{"page":2,"total_pages":2}}`)
		}
	}))
	defer srv.Close()
	c, _ := newCloudflare(map[string]string{"api_token": "tok"})
	c.base = srv.URL + "/client/v4"
	zs, err := c.ListZones(context.Background())
	if err != nil || len(zs) != 2 {
		t.Fatalf("zones=%+v err=%v", zs, err)
	}
}

func TestAliyunEnsureUnchanged(t *testing.T) {
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			io.WriteString(w, `{"DomainRecords":{"Record":[{"RecordId":"r1","RR":"@","Type":"TXT","Value":"v=spf1 mx ~all","TTL":3600}]}}`)
		case "AddDomainRecord", "UpdateDomainRecord":
			writes++
			io.WriteString(w, `{"RecordId":"r1"}`)
		}
	}))
	defer srv.Close()
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"
	res, err := a.EnsureRecords(context.Background(), "example.com", []Record{{Name: "@", Type: "TXT", Value: "v=spf1 mx ~all", TTL: 3600}})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "unchanged" || writes != 0 {
		t.Fatalf("res=%+v writes=%d", res, writes)
	}
}

func TestAliyunListZonesPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("PageNumber") == "1" {
			var b strings.Builder
			b.WriteString(`{"TotalCount":101,"Domains":{"Domain":[`)
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteString(`{"DomainName":"d.com","DomainId":"1"}`)
			}
			b.WriteString(`]}}`)
			io.WriteString(w, b.String())
		} else {
			io.WriteString(w, `{"TotalCount":101,"Domains":{"Domain":[{"DomainName":"e.com","DomainId":"2"}]}}`)
		}
	}))
	defer srv.Close()
	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"
	zs, err := a.ListZones(context.Background())
	if err != nil || len(zs) != 101 {
		t.Fatalf("zones=%d err=%v", len(zs), err)
	}
}

func TestCloudflareDeleteValueMismatch(t *testing.T) {
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Query().Get("name") == "example.com":
			io.WriteString(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)
		case r.Method == "GET":
			io.WriteString(w, `{"success":true,"result":[{"id":"r1","type":"TXT","name":"x.example.com","content":"other"}]}`)
		case r.Method == "DELETE":
			deleted = true
			io.WriteString(w, `{"success":true,"result":{}}`)
		}
	}))
	defer srv.Close()
	c, _ := newCloudflare(map[string]string{"api_token": "tok"})
	c.base = srv.URL + "/client/v4"
	if err := c.DeleteRecord(context.Background(), "example.com", "x", "TXT", "wanted"); err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("value mismatch should not delete")
	}
}
