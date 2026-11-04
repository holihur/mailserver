package handler

import (
	"testing"

	"mailserver/internal/model"
)

func mxValue(t *testing.T, recs []model.DnsRecord) string {
	t.Helper()
	for _, r := range recs {
		if r.Type == "MX" {
			return r.Value
		}
	}
	t.Fatal("no MX record")
	return ""
}

func TestBuildMailRecordsMXTrailingDot(t *testing.T) {
	cases := []string{"mail.example.com", "mail.example.com.", ""}
	for _, host := range cases {
		recs := BuildMailRecords("example.com", "1.2.3.4", host, "", "")
		if got := mxValue(t, recs); got != "mail.example.com." {
			t.Errorf("mailHost=%q: MX=%q, want %q", host, got, "mail.example.com.")
		}
	}
	recs := BuildMailRecords("example.com", "1.2.3.4", "mx.other.net", "dkim", "")
	if got := mxValue(t, recs); got != "mx.other.net." {
		t.Errorf("custom mailHost: MX=%q", got)
	}
}

func TestNormalizeTargetValue(t *testing.T) {
	cases := []struct{ typ, value, zone, want string }{
		{"MX", "mail.example.com", "example.com", "mail.example.com."},
		{"MX", "mail.example.com.", "example.com", "mail.example.com."},
		{"MX", "mail", "example.com", "mail"},
		{"MX", "@", "example.com", "@"},
		{"MX", "", "example.com", ""},
		{"NS", "ns1.example.com", "example.com", "ns1.example.com."},
		{"NS", "ns1", "example.com", "ns1"},
		{"CNAME", "www.example.com", "example.com", "www.example.com."},
		{"CNAME", "www", "example.com", "www"},
		{"SRV", "10 5 443 sip.example.com", "example.com", "10 5 443 sip.example.com."},
		{"SRV", "10 5 443 sip", "example.com", "10 5 443 sip"},
		{"SRV", "broken", "example.com", "broken"},
		{"TXT", "v=spf1 mx ~all", "example.com", "v=spf1 mx ~all"},
		{"A", "1.2.3.4", "example.com", "1.2.3.4"},
	}
	for _, c := range cases {
		if got := normalizeTargetValue(c.typ, c.value, c.zone); got != c.want {
			t.Errorf("normalizeTargetValue(%q,%q,%q)=%q, want %q", c.typ, c.value, c.zone, got, c.want)
		}
	}
}
