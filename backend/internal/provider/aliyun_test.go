package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPercentEncode(t *testing.T) {
	cases := map[string]string{
		"a b":       "a%20b",
		"a+b":       "a%2Bb",
		"/":         "%2F",
		"~._-":      "~._-",
		"v=spf1 mx": "v%3Dspf1%20mx",
		"a@b.com":   "a%40b.com",
		"中文":        "%E4%B8%AD%E6%96%87",
	}
	for in, want := range cases {
		if got := percentEncode(in); got != want {
			t.Errorf("percentEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAliyunSignDeterministic(t *testing.T) {
	p := map[string]string{
		"Action":           "DescribeDomains",
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      "test-key",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   "fixed-nonce",
		"Timestamp":        "2024-01-01T00:00:00Z",
	}
	s1 := aliyunSign(p, "secret")
	s2 := aliyunSign(p, "secret")
	if s1 != s2 || len(s1) == 0 {
		t.Fatalf("signature not deterministic: %q vs %q", s1, s2)
	}
	if s1 == aliyunSign(p, "other") {
		t.Fatal("signature should differ with different secret")
	}
}

func TestAliyunListZonesAndEnsure(t *testing.T) {
	var added []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("Signature") == "" || q.Get("AccessKeyId") == "" {
			t.Errorf("missing auth params: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("Action") {
		case "DescribeDomains":
			fmt.Fprint(w, `{"TotalCount":1,"Domains":{"Domain":[{"DomainName":"example.com","DomainId":"d1"}]}}`)
		case "DescribeDomainRecords":
			fmt.Fprint(w, `{"TotalCount":0,"DomainRecords":{"Record":[]}}`)
		case "AddDomainRecord":
			added = append(added, q)
			fmt.Fprint(w, `{"RecordId":"r1"}`)
		default:
			t.Errorf("unexpected action %q", q.Get("Action"))
		}
	}))
	defer srv.Close()

	a, err := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	if err != nil {
		t.Fatal(err)
	}
	a.base = srv.URL + "/"

	zs, err := a.ListZones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(zs) != 1 || zs[0].Name != "example.com" || zs[0].ID != "d1" {
		t.Fatalf("unexpected zones: %+v", zs)
	}

	res, err := a.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
		{Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none", TTL: 3600},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Action != "created" || res[1].Action != "created" {
		t.Fatalf("unexpected results: %+v", res)
	}
	if len(added) != 2 {
		t.Fatalf("want 2 AddDomainRecord calls, got %d", len(added))
	}
	if added[0].Get("RR") != "@" || added[0].Get("Priority") != "10" || added[0].Get("DomainName") != "example.com" {
		t.Fatalf("bad MX add params: %v", added[0])
	}
	if strings.Contains(added[1].Encode(), "Priority") {
		t.Fatalf("TXT record should not carry Priority: %v", added[1])
	}
}

func TestAliyunEnsureUpdatesExisting(t *testing.T) {
	var updated url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("Action") {
		case "DescribeDomainRecords":
			fmt.Fprint(w, `{"DomainRecords":{"Record":[{"RecordId":"r9","RR":"mail","Type":"A","Value":"1.1.1.1","TTL":600}]}}`)
		case "UpdateDomainRecord":
			updated = q
			fmt.Fprint(w, `{"RecordId":"r9"}`)
		default:
			t.Errorf("unexpected action %q", q.Get("Action"))
		}
	}))
	defer srv.Close()

	a, _ := newAliyun(map[string]string{"access_key_id": "k", "access_key_secret": "s"})
	a.base = srv.URL + "/"

	res, err := a.EnsureRecords(context.Background(), "example.com", []Record{{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 600}})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Action != "updated" || updated.Get("RecordId") != "r9" || updated.Get("Value") != "2.2.2.2" {
		t.Fatalf("expected update, got %+v / %v", res[0], updated)
	}
}
