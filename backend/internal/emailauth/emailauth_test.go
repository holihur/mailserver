package emailauth

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"mailserver/internal/dkim"
)

func fakeLookup(txt map[string]string) Lookup {
	return Lookup{
		TXT: func(d string) ([]string, error) {
			if v, ok := txt[strings.ToLower(d)]; ok {
				return []string{v}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		IP: func(string) ([]net.IP, error) { return nil, fmt.Errorf("no") },
		MX: func(string) ([]*net.MX, error) { return nil, fmt.Errorf("no") },
	}
}

func TestSPF(t *testing.T) {
	ip := net.ParseIP("1.2.3.4")
	l := fakeLookup(map[string]string{"example.com": "v=spf1 ip4:1.2.3.4 -all"})
	if got := SPF(l, ip, "example.com"); got != SPFPass {
		t.Fatalf("命中 ip4 应 pass，得 %s", got)
	}
	if got := SPF(l, net.ParseIP("9.9.9.9"), "example.com"); got != SPFFail {
		t.Fatalf("-all 未命中应 fail，得 %s", got)
	}
	if got := SPF(l, ip, "none.com"); got != SPFNone {
		t.Fatalf("无记录应 none，得 %s", got)
	}
	soft := fakeLookup(map[string]string{"a.com": "v=spf1 ~all"})
	if got := SPF(soft, ip, "a.com"); got != SPFSoftfail {
		t.Fatalf("~all 应 softfail，得 %s", got)
	}
	inc := fakeLookup(map[string]string{
		"a.com": "v=spf1 include:b.com -all",
		"b.com": "v=spf1 ip4:1.2.3.0/24 -all",
	})
	if got := SPF(inc, ip, "a.com"); got != SPFPass {
		t.Fatalf("include 命中应 pass，得 %s", got)
	}
}

func TestDMARC(t *testing.T) {
	l := fakeLookup(map[string]string{"_dmarc.example.com": "v=DMARC1; p=reject; rua=mailto:x@example.com"})
	if p, ok := DMARC(l, "example.com"); !ok || p != "reject" {
		t.Fatalf("DMARC=%q ok=%v", p, ok)
	}
	if _, ok := DMARC(l, "other.com"); ok {
		t.Fatal("无记录应 present=false")
	}
	if !Aligned("mail.example.com", "example.com", true) {
		t.Fatal("relaxed 同组织域应对齐")
	}
	if Aligned("mail.example.com", "example.com", false) {
		t.Fatal("strict 不同域不应齐")
	}
	if !Aligned("a.example.com", "b.example.com", true) {
		t.Fatal("relaxed 同组织域应对齐(2)")
	}
}

func TestVerifyDKIM(t *testing.T) {
	pem, err := dkim.GeneratePEM()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := dkim.LoadPEM("example.com", "dkim", pem)
	if err != nil {
		t.Fatal(err)
	}
	headers := [][2]string{{"From", "a@example.com"}, {"To", "b@test.local"}, {"Subject", "hi"}}
	body := "hello\r\n"
	sig := signer.Sign(headers, body)
	if sig == "" {
		t.Fatal("签名失败")
	}
	raw := "From: a@example.com\r\nTo: b@test.local\r\nSubject: hi\r\n" + sig + "\r\n\r\n" + body
	l := fakeLookup(map[string]string{"dkim._domainkey.example.com": signer.TXT()})

	if got := VerifyDKIM(l, []byte(raw)); got != "pass" {
		t.Fatalf("应 pass，得 %s", got)
	}
	tampered := strings.Replace(raw, "hi", "hacked", 1)
	if got := VerifyDKIM(l, []byte(tampered)); got != "fail" {
		t.Fatalf("被篡改应 fail，得 %s", got)
	}
	if got := VerifyDKIM(l, []byte("From: x@y.z\r\n\r\nbody\r\n")); got != "none" {
		t.Fatalf("无签名应 none，得 %s", got)
	}
}

func TestEvaluate(t *testing.T) {
	pem, _ := dkim.GeneratePEM()
	signer, _ := dkim.LoadPEM("example.com", "dkim", pem)
	headers := [][2]string{{"From", "a@example.com"}, {"To", "b@test.local"}, {"Subject", "hi"}}
	body := "hello\r\n"
	sig := signer.Sign(headers, body)
	raw := "From: a@example.com\r\nTo: b@test.local\r\nSubject: hi\r\n" + sig + "\r\n\r\n" + body
	l := fakeLookup(map[string]string{
		"example.com":                 "v=spf1 ip4:1.2.3.4 -all",
		"_dmarc.example.com":          "v=DMARC1; p=reject",
		"dkim._domainkey.example.com": signer.TXT(),
	})
	res := Evaluate(l, net.ParseIP("1.2.3.4"), "bounce@example.com", []byte(raw))
	if res.SPF != SPFPass || res.DKIM != "pass" || res.DMARC != "pass" {
		t.Fatalf("应全部 pass，得 %+v", res)
	}
	if res.Policy != "reject" {
		t.Fatalf("policy=%q", res.Policy)
	}
	if !strings.Contains(res.String(), "dmarc=pass") {
		t.Fatalf("String=%q", res.String())
	}
	// 无 DMARC 记录 -> none
	noDmarc := fakeLookup(map[string]string{"example.org": "v=spf1 ip4:1.2.3.4 -all"})
	res2 := Evaluate(noDmarc, net.ParseIP("1.2.3.4"), "x@example.org", []byte("From: x@example.org\r\n\r\nhi\r\n"))
	if res2.DMARC != "none" {
		t.Fatalf("无 DMARC 应 none，得 %q", res2.DMARC)
	}
}
