package dnsserver

import (
	"testing"

	"github.com/miekg/dns"
)

func TestDNSSECSign(t *testing.T) {
	EnableDNSSEC(t.TempDir(), false)
	defer EnableDNSSEC("", false)

	keys := (&Server{}).DNSKEYs("example.com")
	if len(keys) != 3 {
		t.Fatalf("应返回 KSK+ZSK+DNSKEY RRSIG，得到 %d", len(keys))
	}
	var zsk *dns.DNSKEY
	for _, rr := range keys {
		if k, ok := rr.(*dns.DNSKEY); ok && k.Flags == 256 {
			zsk = k
		}
	}
	if zsk == nil {
		t.Fatal("缺少 ZSK")
	}
	if DS("example.com") == nil {
		t.Fatal("DS 不应为空")
	}

	a, _ := dns.NewRR("example.com. 3600 IN A 1.2.3.4")
	s := &Server{}
	signed := s.signAnswers("example.com", []dns.RR{a})
	if len(signed) != 2 {
		t.Fatalf("应含 1 条 A + 1 条 RRSIG，得到 %d", len(signed))
	}
	sig, ok := signed[1].(*dns.RRSIG)
	if !ok {
		t.Fatalf("第二条应为 RRSIG，得到 %T", signed[1])
	}
	if err := sig.Verify(zsk, []dns.RR{a}); err != nil {
		t.Fatalf("RRSIG 验签失败: %v", err)
	}
}

func TestNSECNegativeProof(t *testing.T) {
	EnableDNSSEC(t.TempDir(), false)
	defer EnableDNSSEC("", false)
	s := &Server{}
	z := &Zone{Domain: "example.com", Records: []Rec{
		{Name: "@", Type: "A", Value: "1.2.3.4"},
		{Name: "mail", Type: "A", Value: "1.2.3.5"},
	}}

	var zsk *dns.DNSKEY
	for _, rr := range s.DNSKEYs("example.com") {
		if k, ok := rr.(*dns.DNSKEY); ok && k.Flags == 256 {
			zsk = k
		}
	}

	check := func(qname string, wantNX bool) {
		ns := s.negativeProof(z, qname)
		var nsec *dns.NSEC
		var sig *dns.RRSIG
		for _, rr := range ns {
			switch v := rr.(type) {
			case *dns.NSEC:
				nsec = v
			case *dns.RRSIG:
				if v.TypeCovered == dns.TypeNSEC {
					sig = v
				}
			}
		}
		if nsec == nil || sig == nil {
			t.Fatalf("%s: 缺少 NSEC 或其 RRSIG", qname)
		}
		if err := sig.Verify(zsk, []dns.RR{nsec}); err != nil {
			t.Fatalf("%s: NSEC 验签失败: %v", qname, err)
		}
		if wantNX && nsec.Hdr.Name == dns.Fqdn(qname) {
			t.Fatalf("%s: NXDOMAIN 应使用覆盖 NSEC 而非精确匹配", qname)
		}
	}
	check("mail.example.com.", false) // NODATA：精确匹配
	check("nope.example.com.", true)  // NXDOMAIN：覆盖区间
}

func TestNSEC3NegativeProof(t *testing.T) {
	EnableDNSSEC(t.TempDir(), true)
	defer EnableDNSSEC("", false)
	s := &Server{}
	z := &Zone{Domain: "example.com", Records: []Rec{
		{Name: "@", Type: "A", Value: "1.2.3.4"},
		{Name: "mail", Type: "A", Value: "1.2.3.5"},
	}}
	var zsk *dns.DNSKEY
	for _, rr := range s.DNSKEYs("example.com") {
		if k, ok := rr.(*dns.DNSKEY); ok && k.Flags == 256 {
			zsk = k
		}
	}
	for _, q := range []string{"mail.example.com.", "nope.example.com."} {
		ns := s.negativeProof(z, q)
		found := 0
		for i, rr := range ns {
			n3, ok := rr.(*dns.NSEC3)
			if !ok {
				continue
			}
			found++
			if i+1 < len(ns) {
				if sig, ok := ns[i+1].(*dns.RRSIG); ok {
					if err := sig.Verify(zsk, []dns.RR{n3}); err != nil {
						t.Fatalf("%s: NSEC3 验签失败: %v", q, err)
					}
				}
			}
		}
		if found == 0 {
			t.Fatalf("%s: 应含 NSEC3", q)
		}
	}
}

func TestDSForZone(t *testing.T) {
	EnableDNSSEC(t.TempDir(), false)
	defer EnableDNSSEC("", false)
	if !Enabled() {
		t.Fatal("Enabled 应为 true")
	}
	if DSForZone("example.com") == "" {
		t.Fatal("DS 不应为空")
	}
	if n := len(DNSKEYForZone("example.com")); n != 2 {
		t.Fatalf("应有 KSK+ZSK 两条，得到 %d", n)
	}
}

func TestCanonicalLess(t *testing.T) {
	order := []string{"example.com.", "a.example.com.", "mail.example.com.", "z.example.com."}
	for i := 0; i < len(order)-1; i++ {
		if !canonicalLess(order[i], order[i+1]) {
			t.Fatalf("%s 应小于 %s", order[i], order[i+1])
		}
	}
}
