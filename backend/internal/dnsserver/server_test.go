package dnsserver

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "zones.json")
	s := New(p, "ns1.example.com.")
	s.load() // 缺失文件：不 panic
	if len(s.zones) != 0 {
		t.Fatal("zones should be empty")
	}
	if err := os.WriteFile(p, []byte(`{"zones":[{"domain":"example.com","records":[{"name":"@","type":"A","value":"1.2.3.4"}]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	s.load()
	if len(s.zones) != 1 || s.zones[0].Domain != "example.com" {
		t.Fatalf("zones=%+v", s.zones)
	}
	if soa := s.soaRR("example.com"); soa.(*dns.SOA).Ns != "ns1.example.com." {
		t.Fatal("NSHost should override SOA ns")
	}
	_ = os.WriteFile(p, []byte("not json"), 0644)
	s.load() // 非法 JSON：保留旧数据
	if len(s.zones) != 1 {
		t.Fatal("bad json should keep previous zones")
	}
}

func TestFQDN(t *testing.T) {
	cases := []struct{ name, domain, want string }{
		{"@", "example.com", "example.com."},
		{"", "example.com", "example.com."},
		{"mail", "example.com", "mail.example.com."},
		{"mail.example.com.", "example.com", "mail.example.com."},
		{"Mail", "Example.COM", "mail.example.com."},
	}
	for _, c := range cases {
		if got := fqdn(c.name, c.domain); got != c.want {
			t.Errorf("fqdn(%q,%q)=%q want %q", c.name, c.domain, got, c.want)
		}
	}
}

func TestChunkTXT(t *testing.T) {
	if got := chunkTXT(""); len(got) != 1 || got[0] != "" {
		t.Fatalf("empty: %#v", got)
	}
	long := make([]byte, 450)
	for i := range long {
		long[i] = 'a'
	}
	got := chunkTXT(string(long))
	if len(got) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(got))
	}
}

func TestParseCAA(t *testing.T) {
	flag, tag, val := parseCAA(`0 issue "letsencrypt.org"`)
	if flag != 0 || tag != "issue" || val != "letsencrypt.org" {
		t.Fatalf("got %d %q %q", flag, tag, val)
	}
	if _, tag, _ := parseCAA("single"); tag != "issue" {
		t.Fatalf("fallback tag=%q", tag)
	}
}

func sampleServer() *Server {
	return &Server{zones: []Zone{{
		Domain: "example.com",
		Records: []Rec{
			{Name: "@", Type: "A", Value: "1.2.3.4", TTL: 600},
			{Name: "mail", Type: "A", Value: "1.2.3.4"},
			{Name: "@", Type: "MX", Value: "mail.example.com", Prio: 10},
			{Name: "@", Type: "TXT", Value: "v=spf1 mx ~all"},
			{Name: "@", Type: "NS", Value: "ns1.example.com."},
		},
	}}}
}

func TestFindZone(t *testing.T) {
	s := sampleServer()
	if z := s.findZone("mail.example.com."); z == nil || z.Domain != "example.com" {
		t.Fatalf("findZone=%v", z)
	}
	if z := s.findZone("other.org."); z != nil {
		t.Fatalf("expected nil, got %v", z)
	}
}

func TestToRRs(t *testing.T) {
	s := sampleServer()
	z := &s.zones[0]
	if rrs := toRRs(z, dns.TypeA); len(rrs) != 2 {
		t.Fatalf("A rrs=%d", len(rrs))
	}
	if rrs := toRRs(z, dns.TypeMX); len(rrs) != 1 {
		t.Fatalf("MX rrs=%d", len(rrs))
	}
	if rrs := toRRs(z, dns.TypeTXT); len(rrs) != 1 {
		t.Fatalf("TXT rrs=%d", len(rrs))
	}
}

type fakeWriter struct{ msg *dns.Msg }

func (f *fakeWriter) LocalAddr() net.Addr         { return &net.TCPAddr{} }
func (f *fakeWriter) RemoteAddr() net.Addr        { return &net.TCPAddr{} }
func (f *fakeWriter) WriteMsg(m *dns.Msg) error   { f.msg = m; return nil }
func (f *fakeWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeWriter) Close() error                { return nil }
func (f *fakeWriter) TsigStatus() error           { return nil }
func (f *fakeWriter) TsigTimersOnly(bool)         {}
func (f *fakeWriter) Hijack()                     {}

func query(name string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	return m
}

func TestHandle(t *testing.T) {
	s := sampleServer()
	cases := []struct {
		name  string
		qtype uint16
		rcode int
		an    int
	}{
		{"mail.example.com.", dns.TypeA, dns.RcodeSuccess, 1},
		{"example.com.", dns.TypeMX, dns.RcodeSuccess, 1},
		{"example.com.", dns.TypeSOA, dns.RcodeSuccess, 1},
		{"nope.example.com.", dns.TypeA, dns.RcodeNameError, 0},
		{"nope.org.", dns.TypeA, dns.RcodeRefused, 0},
	}
	for _, c := range cases {
		fw := &fakeWriter{}
		s.handle(fw, query(c.name, c.qtype))
		if fw.msg == nil {
			t.Fatalf("%s: no response", c.name)
		}
		if fw.msg.Rcode != c.rcode {
			t.Errorf("%s qtype %d: rcode=%d want %d", c.name, c.qtype, fw.msg.Rcode, c.rcode)
		}
		if len(fw.msg.Answer) != c.an {
			t.Errorf("%s qtype %d: answers=%d want %d", c.name, c.qtype, len(fw.msg.Answer), c.an)
		}
	}

	// 区传送被拒绝
	fw := &fakeWriter{}
	s.handle(fw, query("example.com.", dns.TypeAXFR))
	if fw.msg.Rcode != dns.RcodeRefused {
		t.Fatal("AXFR should be refused")
	}
}
