package dnsserver

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// NSEC3（RFC 5155）：哈希化否定应答，SHA-1、迭代 0、每 zone 随机 salt。

var useNSEC3 bool

func nsec3Salt(zone string) string {
	if dnssecDir == "" {
		return ""
	}
	p := filepath.Join(dnssecDir, zone+".nsec3salt")
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b))
	}
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	s := hex.EncodeToString(buf)
	_ = os.WriteFile(p, []byte(s), 0o600)
	return s
}

// zoneNSEC3 生成 zone 的全部 NSEC3 记录（按哈希排序，环形 next）。
func (s *Server) zoneNSEC3(z *Zone) []*dns.NSEC3 {
	salt := nsec3Salt(strings.ToLower(z.Domain))
	owners := zoneOwners(z)
	apex := dns.Fqdn(strings.ToLower(z.Domain))
	type hr struct{ hash, name string }
	var hs []hr
	for name := range owners {
		hs = append(hs, hr{dns.HashName(name, dns.SHA1, 0, salt), name})
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].hash < hs[j].hash })
	out := make([]*dns.NSEC3, 0, len(hs))
	for i, h := range hs {
		next := hs[(i+1)%len(hs)].hash
		ts := owners[h.name]
		ts[dns.TypeRRSIG] = true
		out = append(out, &dns.NSEC3{
			Hdr:        dns.RR_Header{Name: h.hash + "." + apex, Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: 3600},
			Hash:       dns.SHA1,
			Flags:      0,
			Iterations: 0,
			SaltLength: uint8(len(salt) / 2),
			Salt:       strings.ToLower(salt),
			HashLength: uint8(len(next)),
			NextDomain: next,
			TypeBitMap: bitmap(ts),
		})
	}
	return out
}

// negativeProofNSEC3 生成 NSEC3 否定应答（NODATA / NXDOMAIN）。
func (s *Server) negativeProofNSEC3(z *Zone, qname string) []dns.RR {
	zk := loadKeys(z.Domain)
	soa := s.soaRR(z.Domain)
	out := []dns.RR{soa}
	if zk == nil {
		return out
	}
	if sig := signRRset(z.Domain, zk.zsk, zk.zskPriv, []dns.RR{soa}); sig != nil {
		out = append(out, sig)
	}
	zone := strings.ToLower(z.Domain)
	apex := dns.Fqdn(zone)
	salt := nsec3Salt(zone)
	nsecs := s.zoneNSEC3(z)
	byHash := map[string]*dns.NSEC3{}
	for _, n := range nsecs {
		byHash[strings.TrimSuffix(n.Hdr.Name, "."+apex)] = n
	}
	qname = dns.Fqdn(strings.ToLower(qname))

	proof := map[string]*dns.NSEC3{}
	addMatching := func(name string) {
		if n, ok := byHash[dns.HashName(name, dns.SHA1, 0, salt)]; ok {
			proof[n.Hdr.Name] = n
		}
	}
	addCovering := func(name string) {
		if n := coveringNSEC3(nsecs, apex, dns.HashName(name, dns.SHA1, 0, salt)); n != nil {
			proof[n.Hdr.Name] = n
		}
	}

	if _, ok := byHash[dns.HashName(qname, dns.SHA1, 0, salt)]; ok {
		addMatching(qname) // NODATA：精确匹配
	} else {
		// closest encloser：从 qname 向上找第一个存在的祖先
		ce := qname
		for {
			if _, ok := byHash[dns.HashName(ce, dns.SHA1, 0, salt)]; ok || ce == apex {
				break
			}
			i := strings.IndexByte(ce, '.')
			if i < 0 {
				break
			}
			ce = ce[i+1:]
		}
		// next closer：比 ce 多一个标签、且是 qname 后缀的名字
		nextCloser := qname
		ql := strings.Split(strings.TrimSuffix(qname, "."), ".")
		cl := len(strings.Split(strings.TrimSuffix(ce, "."), "."))
		if len(ql) > cl {
			nextCloser = strings.Join(ql[len(ql)-cl-1:], ".") + "."
		}
		addMatching(ce)
		addCovering(nextCloser)
		addCovering("*." + ce)
	}
	for _, n := range proof {
		out = append(out, n)
		if sig := signRRset(z.Domain, zk.zsk, zk.zskPriv, []dns.RR{n}); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func coveringNSEC3(nsecs []*dns.NSEC3, apex, h string) *dns.NSEC3 {
	if len(nsecs) == 0 {
		return nil
	}
	owner := func(n *dns.NSEC3) string { return strings.TrimSuffix(n.Hdr.Name, "."+apex) }
	first := owner(nsecs[0])
	last := nsecs[len(nsecs)-1]
	if h < first || h > owner(last) {
		return last
	}
	for _, n := range nsecs {
		if owner(n) < h && h < n.NextDomain {
			return n
		}
	}
	return last
}
