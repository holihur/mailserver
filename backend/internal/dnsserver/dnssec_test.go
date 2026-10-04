package dnsserver

import (
	"testing"

	"github.com/miekg/dns"
)

func TestDNSSECSign(t *testing.T) {
	EnableDNSSEC(t.TempDir())
	defer EnableDNSSEC("")

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
