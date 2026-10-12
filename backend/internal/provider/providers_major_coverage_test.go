package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 本文件补齐新增（top）服务商的 Verify / ListZones / DeleteRecord 与错误分支覆盖。

func TestDNSNameHelpers(t *testing.T) {
	if dnsName("") != "@" || dnsName("@") != "@" || dnsName("mail") != "mail" {
		t.Fatal("dnsName")
	}
	if !sameName("", "@") || sameName("mail", "other") {
		t.Fatal("sameName")
	}
	if intPtrOrZero(nil) != 0 {
		t.Fatal("intPtrOrZero(nil)")
	}
	n := 7
	if intPtrOrZero(&n) != 7 {
		t.Fatal("intPtrOrZero(&n)")
	}
}

func TestDigitalOceanVerifyListZonesDeleteErrors(t *testing.T) {
	var deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.Method == "GET" && r.URL.Path == "/domains":
			if q.Get("per_page") == "1" {
				_, _ = io.WriteString(w, `{"domains":[{"name":"a.com"}]}`)
				return
			}
			if q.Get("page") == "1" {
				var b strings.Builder
				b.WriteString(`{"domains":[`)
				for i := 0; i < 200; i++ {
					if i > 0 {
						b.WriteByte(',')
					}
					b.WriteString(`{"name":"a.com"}`)
				}
				b.WriteString(`]}`)
				_, _ = io.WriteString(w, b.String())
				return
			}
			_, _ = io.WriteString(w, `{"domains":[{"name":"b.com"}]}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			_, _ = io.WriteString(w, `{"domain_records":[{"id":1,"type":"TXT","name":"x","data":"v","ttl":30}]}`)
		case r.Method == "DELETE":
			deletes++
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	d, _ := newDigitalOcean(map[string]string{"api_token": "t"})
	d.base = srv.URL
	if err := d.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	zs, err := d.ListZones(context.Background())
	if err != nil || len(zs) != 201 {
		t.Fatalf("zones=%d err=%v", len(zs), err)
	}
	if err := d.DeleteRecord(context.Background(), "a.com", "x", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("deletes=%d", deletes)
	}
	if err := d.DeleteRecord(context.Background(), "a.com", "x", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("mismatch should not delete, deletes=%d", deletes)
	}
	if err := d.DeleteRecord(context.Background(), "a.com", "nope", "TXT", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalOceanErrorsAndUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"message":"bad token"}`)
	}))
	defer srv.Close()
	d, _ := newDigitalOcean(map[string]string{"api_token": "t"})
	d.base = srv.URL
	if err := d.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("err=%v", err)
	}
	if _, err := d.ListZones(context.Background()); err == nil {
		t.Fatal("ListZones should error")
	}
	if _, err := d.EnsureRecords(context.Background(), "a.com", []Record{{Name: "x", Type: "TXT", Value: "v"}}); err != nil {
		t.Fatal(err)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"domain_records":[{"id":1,"type":"A","name":"mail","data":"1.2.3.4","ttl":1800}]}`)
	}))
	defer srv2.Close()
	d2, _ := newDigitalOcean(map[string]string{"api_token": "t"})
	d2.base = srv2.URL
	res, err := d2.EnsureRecords(context.Background(), "a.com", []Record{{Name: "mail", Type: "A", Value: "1.2.3.4", TTL: 1800}})
	if err != nil || res[0].Action != "unchanged" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestVultrVerifyListZonesDeleteErrors(t *testing.T) {
	var deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v2/domains":
			if r.URL.Query().Get("per_page") == "1" {
				_, _ = io.WriteString(w, `{"domains":[{"domain":"a.com"}],"meta":{"links":{"next":""}}}`)
				return
			}
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"domains":[{"domain":"a.com"}],"meta":{"links":{"next":"/v2/domains?cursor=abc"}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"domains":[{"domain":"b.com"}],"meta":{"links":{"next":""}}}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			_, _ = io.WriteString(w, `{"records":[{"id":"r1","type":"TXT","name":"x","data":"v","ttl":300,"priority":0}],"meta":{"links":{"next":""}}}`)
		case r.Method == "DELETE":
			deletes++
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	v, _ := newVultr(map[string]string{"api_token": "t"})
	v.base = srv.URL
	if err := v.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	zs, err := v.ListZones(context.Background())
	if err != nil || len(zs) != 2 {
		t.Fatalf("zones=%+v err=%v", zs, err)
	}
	if err := v.DeleteRecord(context.Background(), "a.com", "x", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("deletes=%d", deletes)
	}
	if err := v.DeleteRecord(context.Background(), "a.com", "x", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("mismatch should not delete, deletes=%d", deletes)
	}
}

func TestVultrErrorsAndUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"error":"nope"}`)
	}))
	defer srv.Close()
	v, _ := newVultr(map[string]string{"api_token": "t"})
	v.base = srv.URL
	if err := v.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err=%v", err)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"records":[{"id":"r1","type":"A","name":"mail","data":"1.2.3.4","ttl":300,"priority":0}],"meta":{"links":{"next":""}}}`)
	}))
	defer srv2.Close()
	v2, _ := newVultr(map[string]string{"api_token": "t"})
	v2.base = srv2.URL
	res, err := v2.EnsureRecords(context.Background(), "a.com", []Record{{Name: "mail", Type: "A", Value: "1.2.3.4", TTL: 300}})
	if err != nil || res[0].Action != "unchanged" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestLinodeVerifyListZonesDeleteErrors(t *testing.T) {
	var deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/domains":
			_, _ = io.WriteString(w, `{"data":[{"id":5,"domain":"a.com"}],"pages":1}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records"):
			_, _ = io.WriteString(w, `{"data":[{"id":1,"type":"TXT","name":"x","target":"v","ttl_sec":300,"priority":0}],"pages":1}`)
		case r.Method == "DELETE":
			deletes++
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	l, _ := newLinode(map[string]string{"api_token": "t"})
	l.base = srv.URL
	if err := l.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	zs, err := l.ListZones(context.Background())
	if err != nil || len(zs) != 1 || zs[0].ID != "5" {
		t.Fatalf("zones=%+v err=%v", zs, err)
	}
	if err := l.DeleteRecord(context.Background(), "a.com", "x", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("deletes=%d", deletes)
	}
	if err := l.DeleteRecord(context.Background(), "a.com", "x", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("mismatch should not delete, deletes=%d", deletes)
	}
}

func TestLinodeErrorsAndZoneMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"errors":[{"reason":"bad token"}]}`)
	}))
	defer srv.Close()
	l, _ := newLinode(map[string]string{"api_token": "t"})
	l.base = srv.URL
	if err := l.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("err=%v", err)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"pages":1}`)
	}))
	defer srv2.Close()
	l2, _ := newLinode(map[string]string{"api_token": "t"})
	l2.base = srv2.URL
	if _, err := l2.EnsureRecords(context.Background(), "nope.com", []Record{{Name: "@", Type: "TXT", Value: "v"}}); err == nil {
		t.Fatal("missing zone should error")
	}

	// 非 JSON 响应 -> 解析失败
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not json`)
	}))
	defer srv3.Close()
	l3, _ := newLinode(map[string]string{"api_token": "t"})
	l3.base = srv3.URL
	if _, err := l3.ListZones(context.Background()); err == nil {
		t.Fatal("bad json should error")
	}
}

func TestGoDaddyVerifyDeleteAndRRset(t *testing.T) {
	var deletes, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/domains":
			_, _ = io.WriteString(w, `[{"domainId":"1","domain":"a.com"}]`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/records/"):
			_, _ = io.WriteString(w, `[{"data":"v","name":"x","ttl":600,"type":"TXT"},{"data":"w","name":"x","ttl":600,"type":"TXT"}]`)
		case r.Method == "DELETE":
			deletes++
			_, _ = io.WriteString(w, `{}`)
		case r.Method == "PUT":
			puts++
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	g, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	g.base = srv.URL
	if err := g.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	// value 为空 -> 删除整组
	if err := g.DeleteRecord(context.Background(), "a.com", "x", "TXT", ""); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("deletes=%d", deletes)
	}
	// value 命中其一 -> 保留其余并 PUT
	if err := g.DeleteRecord(context.Background(), "a.com", "x", "TXT", "v"); err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Fatalf("puts=%d", puts)
	}
	// value 不命中 -> 不改
	if err := g.DeleteRecord(context.Background(), "a.com", "x", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Fatalf("puts=%d", puts)
	}
	// 已存在同值 -> unchanged
	res, err := g.EnsureRecords(context.Background(), "a.com", []Record{{Name: "x", Type: "TXT", Value: "v", TTL: 600}})
	if err != nil || res[0].Action != "unchanged" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	// 新值 -> 替换 RRset 首条
	res, err = g.EnsureRecords(context.Background(), "a.com", []Record{{Name: "x", Type: "TXT", Value: "new", TTL: 600}})
	if err != nil || res[0].Action != "updated" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestGoDaddyErrorsAndNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"code":"BAD","message":"nope"}`)
	}))
	defer srv.Close()
	g, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	g.base = srv.URL
	if err := g.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "BAD") {
		t.Fatalf("err=%v", err)
	}

	// 记录不存在（404）视为空 -> created
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"code":"NOT_FOUND","message":"missing"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv2.Close()
	g2, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	g2.base = srv2.URL
	res, err := g2.EnsureRecords(context.Background(), "a.com", []Record{{Name: "mail", Type: "A", Value: "1.2.3.4", TTL: 600}})
	if err != nil || res[0].Action != "created" {
		t.Fatalf("res=%+v err=%v", res, err)
	}

	// 非 JSON 响应 -> 解析失败
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `nope`)
	}))
	defer srv3.Close()
	g3, _ := newGoDaddy(map[string]string{"api_key": "k", "api_secret": "s"})
	g3.base = srv3.URL
	if err := g3.Verify(context.Background()); err == nil {
		t.Fatal("bad json should error")
	}
}

func TestTencentDeleteMismatchAndErrors(t *testing.T) {
	var deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			_, _ = io.WriteString(w, `{"Response":{"RecordList":[{"RecordId":3,"Name":"x","Type":"TXT","Value":"v","TTL":600}]}}`)
		case "DeleteRecord":
			deletes++
			_, _ = io.WriteString(w, `{"Response":{"RequestId":"x"}}`)
		default:
			_, _ = io.WriteString(w, `{"Response":{}}`)
		}
	}))
	defer srv.Close()
	tt, _ := newTencent(map[string]string{"secret_id": "id", "secret_key": "key"})
	tt.base = srv.URL
	if err := tt.DeleteRecord(context.Background(), "a.com", "x", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
	if deletes != 0 {
		t.Fatal("mismatch should not delete")
	}
	// 找不到 -> 不报错
	if err := tt.DeleteRecord(context.Background(), "a.com", "nope", "TXT", ""); err != nil {
		t.Fatal(err)
	}
}
