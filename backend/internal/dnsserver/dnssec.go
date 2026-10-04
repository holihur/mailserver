package dnsserver

import (
	"crypto"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSSEC：为托管 zone 生成/加载 KSK+ZSK，对正向应答签名（RRSIG），并提供 DS。
// 密钥文件存 DATA_DIR/dnssec/<zone>.{ksk,zsk}.{key,private}（BIND 格式）。

type zoneKeys struct {
	ksk, zsk         *dns.DNSKEY
	kskPriv, zskPriv crypto.Signer
}

var (
	dnssecDir string
	keyMu     sync.Mutex
	keyCache  = map[string]*zoneKeys{}
)

// EnableDNSSEC 启用 DNSSEC，dir 为密钥目录（空=禁用）。
func EnableDNSSEC(dir string) { dnssecDir = dir }

func loadKeys(zone string) *zoneKeys {
	if dnssecDir == "" {
		return nil
	}
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	keyMu.Lock()
	defer keyMu.Unlock()
	if zk, ok := keyCache[zone]; ok {
		return zk
	}
	zk, err := generateOrLoad(zone)
	if err != nil {
		log.Println("dnssec key:", err)
		keyCache[zone] = nil
		return nil
	}
	keyCache[zone] = zk
	return zk
}

func generateOrLoad(zone string) (*zoneKeys, error) {
	if err := os.MkdirAll(dnssecDir, 0o700); err != nil {
		return nil, err
	}
	name := dns.Fqdn(zone)
	ksk := &dns.DNSKEY{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600}, Flags: 257, Protocol: 3, Algorithm: dns.RSASHA256}
	zsk := &dns.DNSKEY{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600}, Flags: 256, Protocol: 3, Algorithm: dns.RSASHA256}
	kskPriv, err := loadOrGen(ksk, filepath.Join(dnssecDir, zone+".ksk"))
	if err != nil {
		return nil, err
	}
	zskPriv, err := loadOrGen(zsk, filepath.Join(dnssecDir, zone+".zsk"))
	if err != nil {
		return nil, err
	}
	return &zoneKeys{ksk: ksk, zsk: zsk, kskPriv: kskPriv, zskPriv: zskPriv}, nil
}

func loadOrGen(k *dns.DNSKEY, base string) (crypto.Signer, error) {
	if b, err := os.ReadFile(base + ".private"); err == nil {
		if priv, err := k.NewPrivateKey(string(b)); err == nil {
			if s, ok := priv.(crypto.Signer); ok {
				return s, nil
			}
		}
	}
	priv, err := k.Generate(2048)
	if err != nil {
		return nil, err
	}
	s, ok := priv.(crypto.Signer)
	if !ok {
		return nil, errors.New("dnssec: key not a signer")
	}
	_ = os.WriteFile(base+".private", []byte(k.PrivateKeyString(priv)), 0o600)
	_ = os.WriteFile(base+".key", []byte(k.String()+"\n"), 0o644)
	return s, nil
}

// DNSKEYs 返回 zone 的 KSK+ZSK（并附 KSK 对 DNSKEY 的 RRSIG）。
func (s *Server) DNSKEYs(zone string) []dns.RR {
	zk := loadKeys(zone)
	if zk == nil {
		return nil
	}
	set := []dns.RR{zk.ksk, zk.zsk}
	if sig := signRRset(zone, zk.ksk, zk.kskPriv, set); sig != nil {
		set = append(set, sig)
	}
	return set
}

// DS 返回注册商应设置的 DS 记录（SHA-256）。
func DS(zone string) *dns.DS {
	zk := loadKeys(zone)
	if zk == nil {
		return nil
	}
	return zk.ksk.ToDS(dns.SHA256)
}

// signAnswers 对一组应答按 name+type 分组做 RRSIG。
func (s *Server) signAnswers(zone string, answers []dns.RR) []dns.RR {
	zk := loadKeys(zone)
	if zk == nil || len(answers) == 0 {
		return answers
	}
	groups := map[string][]dns.RR{}
	var order []string
	for _, rr := range answers {
		h := rr.Header()
		key := h.Name + "|" + dns.TypeToString[h.Rrtype]
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], rr)
	}
	out := make([]dns.RR, 0, len(answers)*2)
	for _, key := range order {
		set := groups[key]
		out = append(out, set...)
		if sig := signRRset(zone, zk.zsk, zk.zskPriv, set); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

// canonicalLess 按 RFC 4034 规范序比较域名（小写、从右向左逐标签）。
func canonicalLess(a, b string) bool {
	la := strings.Split(strings.ToLower(strings.TrimSuffix(a, ".")), ".")
	lb := strings.Split(strings.ToLower(strings.TrimSuffix(b, ".")), ".")
	i, j := len(la)-1, len(lb)-1
	for i >= 0 && j >= 0 {
		if la[i] != lb[j] {
			return la[i] < lb[j]
		}
		i--
		j--
	}
	return i < j
}

// zoneNSEC 生成 zone 的全部 NSEC 记录（已排序，环形 next）。
func (s *Server) zoneNSEC(z *Zone) []*dns.NSEC {
	types := map[string]map[uint16]bool{}
	add := func(name string, t uint16) {
		name = dns.Fqdn(strings.ToLower(name))
		if types[name] == nil {
			types[name] = map[uint16]bool{}
		}
		types[name][t] = true
	}
	apex := dns.Fqdn(strings.ToLower(z.Domain))
	add(apex, dns.TypeSOA)
	add(apex, dns.TypeNS)
	add(apex, dns.TypeDNSKEY)
	for _, r := range z.Records {
		name := fqdn(r.Name, z.Domain)
		switch strings.ToUpper(r.Type) {
		case "A":
			add(name, dns.TypeA)
		case "AAAA":
			add(name, dns.TypeAAAA)
		case "MX":
			add(name, dns.TypeMX)
		case "TXT":
			add(name, dns.TypeTXT)
		case "NS":
			add(name, dns.TypeNS)
		case "CNAME":
			add(name, dns.TypeCNAME)
		case "SRV":
			add(name, dns.TypeSRV)
		case "CAA":
			add(name, dns.TypeCAA)
		}
	}
	names := make([]string, 0, len(types))
	for n := range types {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return canonicalLess(names[i], names[j]) })
	out := make([]*dns.NSEC, 0, len(names))
	for i, n := range names {
		next := names[(i+1)%len(names)]
		ts := types[n]
		ts[dns.TypeNSEC] = true
		ts[dns.TypeRRSIG] = true
		bm := make([]uint16, 0, len(ts))
		for t := range ts {
			bm = append(bm, t)
		}
		sort.Slice(bm, func(a, b int) bool { return bm[a] < bm[b] })
		out = append(out, &dns.NSEC{
			Hdr:        dns.RR_Header{Name: n, Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: 3600},
			NextDomain: next,
			TypeBitMap: bm,
		})
	}
	return out
}

// negativeProof 生成否定应答的 authority：SOA + SOA RRSIG + NSEC + NSEC RRSIG。
func (s *Server) negativeProof(z *Zone, qname string) []dns.RR {
	zk := loadKeys(z.Domain)
	soa := s.soaRR(z.Domain)
	out := []dns.RR{soa}
	if zk == nil {
		return out // 未启用 DNSSEC：仅 SOA
	}
	if sig := signRRset(z.Domain, zk.zsk, zk.zskPriv, []dns.RR{soa}); sig != nil {
		out = append(out, sig)
	}
	qname = dns.Fqdn(strings.ToLower(qname))
	nsecs := s.zoneNSEC(z)
	var proof *dns.NSEC
	for _, n := range nsecs { // NODATA：精确匹配
		if n.Hdr.Name == qname {
			proof = n
			break
		}
	}
	if proof == nil { // NXDOMAIN：找覆盖区间
		for _, n := range nsecs {
			if canonicalLess(n.Hdr.Name, qname) && canonicalLess(qname, n.NextDomain) {
				proof = n
				break
			}
		}
		if proof == nil && len(nsecs) > 0 {
			last := nsecs[len(nsecs)-1]
			if canonicalLess(last.Hdr.Name, qname) {
				proof = last
			}
		}
	}
	if proof != nil {
		out = append(out, proof)
		if sig := signRRset(z.Domain, zk.zsk, zk.zskPriv, []dns.RR{proof}); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func signRRset(zone string, key *dns.DNSKEY, priv crypto.Signer, set []dns.RR) *dns.RRSIG {
	if len(set) == 0 {
		return nil
	}
	h := set[0].Header()
	labels := dns.CountLabel(h.Name)
	sig := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: h.Name, Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: h.Ttl},
		TypeCovered: h.Rrtype,
		Algorithm:   key.Algorithm,
		Labels:      uint8(labels),
		OrigTtl:     h.Ttl,
		Expiration:  uint32(time.Now().Add(7 * 24 * time.Hour).Unix()),
		Inception:   uint32(time.Now().Add(-time.Hour).Unix()),
		KeyTag:      key.KeyTag(),
		SignerName:  dns.Fqdn(zone),
	}
	if err := sig.Sign(priv, set); err != nil {
		log.Println("dnssec sign:", err)
		return nil
	}
	return sig
}
