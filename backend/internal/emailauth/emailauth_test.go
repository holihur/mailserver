package emailauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
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

// RFC 6376 3.4.4：relaxed body 需把行内连续空白压成单个空格、删行尾空白、去末尾空行。
func TestCanonBodyRelaxedRFC(t *testing.T) {
	// 例子（3.4.5）："<SP>C<SP><CRLF>D<SP><HTAB><SP>E<CRLF><CRLF><CRLF>"
	in := " C \r\nD \t E\r\n\r\n\r\n"
	want := " C\r\nD E\r\n"
	if got := canonBodyRelaxed(in); got != want {
		t.Fatalf("canonBodyRelaxed=%q 期望 %q（Gmail 用 relaxed body，☝ 修复前会 DKIM fail）", got, want)
	}
	if got := canonBodyRelaxed(""); got != "" {
		t.Fatalf("空体应为空，得到 %q", got)
	}
}

// 独立实现的 relaxed body（用不同写法交叉校验）。
func relaxedBodyIndependent(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		for strings.Contains(l, "  ") || strings.Contains(l, "\t") {
			l = strings.ReplaceAll(l, "  ", " ")
			l = strings.ReplaceAll(l, "\t", " ")
		}
		lines[i] = strings.TrimRight(l, " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

// 手写独立签名器：c=relaxed/relaxed，验证器必须能验过（含行内多空格与 Tab）。
func TestVerifyDKIMRelaxedRelaxed(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	txt := "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)

	headers := [][2]string{
		{"From", "alice@gmail.com"},
		{"To", "bob@example.com"},
		{"Subject", "hi"},
		{"Date", "Fri, 11 Jul 2003 21:00:37 -0700"},
	}
	body := "line1   with   spaces\r\n\tindented line\r\n\r\n"

	bh := sha256.Sum256([]byte(relaxedBodyIndependent(body)))
	names := []string{"from", "to", "subject", "date"}
	dk := "v=1; a=rsa-sha256; c=relaxed/relaxed; d=gmail.com; s=20230601; h=" +
		strings.Join(names, ":") + "; bh=" + base64.StdEncoding.EncodeToString(bh[:]) + "; b="

	var input strings.Builder
	for _, h := range headers {
		input.WriteString(strings.ToLower(h[0]) + ":" + strings.Join(strings.Fields(h[1]), " ") + "\r\n")
	}
	input.WriteString("dkim-signature:" + strings.Join(strings.Fields(dk), " ") + "\r\n")
	hh := sha256.Sum256([]byte(input.String()))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hh[:])
	if err != nil {
		t.Fatal(err)
	}

	var raw strings.Builder
	for _, h := range headers {
		raw.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	raw.WriteString("DKIM-Signature: " + dk + base64.StdEncoding.EncodeToString(sig) + "\r\n\r\n" + body)

	l := fakeLookup(map[string]string{"20230601._domainkey.gmail.com": txt})
	if got := VerifyDKIM(l, []byte(raw.String())); got != "pass" {
		t.Fatalf("relaxed/relaxed 应 pass，得到 %s", got)
	}
}
