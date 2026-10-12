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

func TestTencentSignDeterministic(t *testing.T) {
	payload := []byte(`{"Offset":0,"Limit":1}`)
	s1 := tencentSign("id", "key", "dnspod.tencentcloudapi.com", payload, 1700000000)
	s2 := tencentSign("id", "key", "dnspod.tencentcloudapi.com", payload, 1700000000)
	if s1 != s2 || !strings.HasPrefix(s1, "TC3-HMAC-SHA256 Credential=id/") {
		t.Fatalf("bad signature: %q vs %q", s1, s2)
	}
	if s1 == tencentSign("id", "other", "dnspod.tencentcloudapi.com", payload, 1700000000) {
		t.Fatal("signature should differ with different secret")
	}
}

func TestTencentTTL(t *testing.T) {
	cases := map[int]int{0: 1, 1: 1, 30: 60, 60: 60, 100: 120, 3600: 3600, 999999: 86400}
	for in, want := range cases {
		if got := tencentTTL(in); got != want {
			t.Errorf("tencentTTL(%d)=%d want %d", in, got, want)
		}
	}
}

func TestTencentMissingCreds(t *testing.T) {
	if _, err := newTencent(map[string]string{}); err == nil {
		t.Fatal("missing creds should error")
	}
}

func TestTencentListAndEnsure(t *testing.T) {
	var created, modified []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 ") {
			t.Errorf("missing auth header: %q", r.Header.Get("Authorization"))
		}
		action := r.Header.Get("X-TC-Action")
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch action {
		case "DescribeDomainList":
			_, _ = io.WriteString(w, `{"Response":{"DomainList":[{"DomainId":123,"Name":"example.com"}],"RequestId":"x"}}`)
		case "DescribeRecordList":
			// 已存在 mail A -> 1.1.1.1
			_, _ = io.WriteString(w, `{"Response":{"RecordList":[{"RecordId":9,"Name":"mail","Type":"A","Value":"1.1.1.1","TTL":600,"MX":0,"Line":"默认"}]}}`)
		case "CreateRecord":
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			created = append(created, m)
			_, _ = io.WriteString(w, `{"Response":{"RecordId":10,"RequestId":"x"}}`)
		case "ModifyRecord":
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			modified = append(modified, m)
			_, _ = io.WriteString(w, `{"Response":{"RecordId":9,"RequestId":"x"}}`)
		default:
			t.Errorf("unexpected action %q", action)
			_, _ = io.WriteString(w, `{"Response":{"Error":{"Code":"X","Message":"unexpected"}}}`)
		}
	}))
	defer srv.Close()

	tt, err := newTencent(map[string]string{"secret_id": "id", "secret_key": "key"})
	if err != nil {
		t.Fatal(err)
	}
	tt.base = srv.URL

	zs, err := tt.ListZones(context.Background())
	if err != nil || len(zs) != 1 || zs[0].ID != "123" || zs[0].Name != "example.com" {
		t.Fatalf("ListZones: %+v %v", zs, err)
	}

	res, err := tt.EnsureRecords(context.Background(), "example.com", []Record{
		{Name: "mail", Type: "A", Value: "2.2.2.2", TTL: 600},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Action != "updated" || res[1].Action != "created" {
		t.Fatalf("unexpected results: %+v", res)
	}
	if len(modified) != 1 || modified[0]["RecordId"] != float64(9) || modified[0]["Value"] != "2.2.2.2" {
		t.Fatalf("bad modify: %+v", modified)
	}
	if len(created) != 1 || created[0]["SubDomain"] != "@" || created[0]["RecordType"] != "MX" || created[0]["MX"] != float64(10) {
		t.Fatalf("bad create: %+v", created)
	}
}

func TestTencentUnchangedAndDelete(t *testing.T) {
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			_, _ = io.WriteString(w, `{"Response":{"RecordList":[{"RecordId":7,"Name":"_dmarc","Type":"TXT","Value":"v=DMARC1","TTL":600}]}}`)
		case "DeleteRecord":
			deleted = true
			_, _ = io.WriteString(w, `{"Response":{"RequestId":"x"}}`)
		default:
			t.Errorf("unexpected action %q", r.Header.Get("X-TC-Action"))
			_, _ = io.WriteString(w, `{"Response":{}}`)
		}
	}))
	defer srv.Close()

	tt, _ := newTencent(map[string]string{"secret_id": "id", "secret_key": "key"})
	tt.base = srv.URL

	res, err := tt.EnsureRecords(context.Background(), "example.com", []Record{{Name: "_dmarc", Type: "TXT", Value: "v=DMARC1", TTL: 600}})
	if err != nil || res[0].Action != "unchanged" {
		t.Fatalf("expected unchanged, got %+v %v", res, err)
	}
	if err := tt.DeleteRecord(context.Background(), "example.com", "_dmarc", "TXT", "v=DMARC1"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected delete call")
	}
	// value 不匹配时不删除
	if err := tt.DeleteRecord(context.Background(), "example.com", "_dmarc", "TXT", "other"); err != nil {
		t.Fatal(err)
	}
}

func TestTencentAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Response":{"Error":{"Code":"AuthFailure.SecretIdNotFound","Message":"bad id"},"RequestId":"x"}}`)
	}))
	defer srv.Close()

	tt, _ := newTencent(map[string]string{"secret_id": "id", "secret_key": "key"})
	tt.base = srv.URL
	if err := tt.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "bad id") {
		t.Fatalf("verify error=%v", err)
	}
}
