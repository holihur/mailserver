// 自研权威 DNS 服务器（低内存：常驻约 8~15MB）。
// 从 zones.json 加载 zones（由 backend API 导出），应答本地域名的 A/AAAA/MX/TXT/NS/SOA/CNAME/SRV/CAA。
// 非本地域名 -> REFUSED（不做递归，防放大攻击）；文件每 10s 检查 mtime 热加载。

package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type Rec struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
	Prio  int    `json:"prio"`
}
type Zone struct {
	Domain  string `json:"domain"`
	Records []Rec  `json:"records"`
}

var (
	mu     sync.RWMutex
	zones  []Zone
	zonesF = getenv("ZONES_PATH", "./zones.json")
	nsHost = getenv("NS_HOST", "") // 如 ns1.example.com.，为空则用各 zone 的 ns1
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loadZones() {
	b, err := os.ReadFile(zonesF)
	if err != nil {
		log.Println("dns: 无 zones 文件:", zonesF, "（等 backend 生成）")
		return
	}
	var v struct {
		Zones []Zone `json:"zones"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		log.Println("dns: zones 解析失败:", err)
		return
	}
	mu.Lock()
	zones = v.Zones
	mu.Unlock()
	log.Printf("dns: 加载 %d 个 zone", len(v.Zones))
}

func watchZones() {
	var last time.Time
	if fi, err := os.Stat(zonesF); err == nil {
		last = fi.ModTime()
	}
	for range time.Tick(10 * time.Second) {
		fi, err := os.Stat(zonesF)
		if err != nil {
			continue
		}
		if fi.ModTime().After(last) {
			last = fi.ModTime()
			loadZones()
		}
	}
}

func fqdn(name, domain string) string {
	name = strings.TrimSpace(name)
	if name == "@" || name == "" {
		return strings.ToLower(domain + ".")
	}
	if strings.HasSuffix(name, ".") {
		return strings.ToLower(name)
	}
	return strings.ToLower(name + "." + domain + ".")
}

// 找最长后缀匹配的 zone
func findZone(q string) *Zone {
	mu.RLock()
	defer mu.RUnlock()
	q = strings.ToLower(q)
	var best *Zone
	for i := range zones {
		suf := strings.ToLower(zones[i].Domain) + "."
		if q == suf || strings.HasSuffix(q, "."+suf) {
			if best == nil || len(zones[i].Domain) > len(best.Domain) {
				b := zones[i]
				best = &b
			}
		}
	}
	return best
}

func soaRR(zone string) dns.RR {
	ttl := uint32(3600)
	s := &dns.SOA{
		Hdr:     dns.RR_Header{Name: zone + ".", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl},
		Ns:      "ns1." + zone + ".",
		Mbox:    "hostmaster." + zone + ".",
		Serial:  1,
		Refresh: 7200, Retry: 3600, Expire: 1209600, Minttl: 300,
	}
	if nsHost != "" {
		s.Ns = nsHost
	}
	return s
}

func toRRs(z *Zone, qtype uint16) []dns.RR {
	var out []dns.RR
	for _, r := range z.Records {
		t := r.Type
		ttl := uint32(r.TTL)
		if ttl == 0 {
			ttl = 600
		}
		name := fqdn(r.Name, z.Domain)
		switch {
		case qtype == dns.TypeA && t == "A":
			if ip := net.ParseIP(r.Value); ip != nil {
				out = append(out, &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ip})
			}
		case qtype == dns.TypeAAAA && t == "AAAA":
			if ip := net.ParseIP(r.Value); ip != nil {
				out = append(out, &dns.AAAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl}, AAAA: ip})
			}
		case qtype == dns.TypeMX && t == "MX":
			mx := r.Value
			if !strings.HasSuffix(mx, ".") {
				mx += "." + z.Domain + "."
			}
			out = append(out, &dns.MX{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: ttl}, Preference: uint16(r.Prio), Mx: mx})
		case qtype == dns.TypeTXT && t == "TXT":
			out = append(out, &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl}, Txt: chunkTXT(r.Value)})
		case qtype == dns.TypeNS && t == "NS":
			ns := r.Value
			if !strings.HasSuffix(ns, ".") {
				ns = fqdn(ns, z.Domain)
			}
			out = append(out, &dns.NS{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}, Ns: ns})
		case qtype == dns.TypeCNAME && t == "CNAME":
			tgt := r.Value
			if !strings.HasSuffix(tgt, ".") {
				tgt = fqdn(tgt, z.Domain)
			}
			out = append(out, &dns.CNAME{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl}, Target: tgt})
		case qtype == dns.TypeSRV && t == "SRV":
			// value: "prio weight port target"
			n, _ := splitSRV(r.Value)
			if len(n) == 4 {
				pr, we, po = atoi(n[0]), atoi(n[1]), atoi(n[2])
				tgt = n[3]
				if !strings.HasSuffix(tgt, ".") {
					tgt = fqdn(tgt, z.Domain)
				}
				out = append(out, &dns.SRV{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: ttl}, Priority: uint16(pr), Weight: uint16(we), Port: uint16(po), Target: tgt})
			}
		case qtype == dns.TypeCAA && t == "CAA":
			// value: '0 issue "letsencrypt.org"' 或整串
			flag, tag, val := parseCAA(r.Value)
			out = append(out, &dns.CAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: ttl}, Flag: flag, Tag: tag, Value: val})
		}
	}
	// 只保留 qname 匹配的（@ 展开后比较）
	return out
}

// TXT 单段 ≤255 字节才合法（如 2048 位 DKIM 公钥 ~400 字符必须切分）
func chunkTXT(s string) []string {
	var out []string
	for len(s) > 0 {
		n := 200
		if len(s) < n {
			n = len(s)
		}
		out = append(out, s[:n])
		s = s[n:]
	}
	if out == nil {
		out = []string{""}
	}
	return out
}

func splitSRV(s string) ([]string, int) {
	f := strings.Fields(s)
	return f, len(f)
}
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
func parseCAA(s string) (uint8, string, string) {
	f := strings.Fields(s)
	if len(f) >= 3 {
		return uint8(atoi(f[0])), f[1], strings.Trim(strings.Join(f[2:], " "), `"`)
	}
	return 0, "issue", strings.Trim(s, `"`)
}
func handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	resp.RecursionAvailable = false
	if len(req.Question) == 0 {
		w.WriteMsg(resp)
		return
	}
	q := req.Question[0]
	z := findZone(q.Name)
	if z == nil {
		resp.Rcode = dns.RcodeRefused // 非托管域：拒绝（不递归）
		w.WriteMsg(resp)
		return
	}
	switch q.Qtype {
	case dns.TypeSOA:
		resp.Answer = []dns.RR{soaRR(z.Domain)}
	case dns.TypeAXFR, dns.TypeIXFR:
		resp.Rcode = dns.RcodeRefused // 禁止区传送
	default:
		ans := []dns.RR{}
		for _, rr := range toRRs(z, q.Qtype) {
			if strings.EqualFold(rr.Header().Name, q.Name) {
				ans = append(ans, rr)
			}
		}
		if len(ans) == 0 {
			// NODATA / NXDOMAIN 判定
			exists := false
			for _, r := range z.Records {
				if strings.EqualFold(fqdn(r.Name, z.Domain), q.Name) {
					exists = true
					break
				}
			}
			if !exists {
				resp.Rcode = dns.RcodeNameError
			}
			resp.Ns = []dns.RR{soaRR(z.Domain)}
		} else {
			resp.Answer = ans
		}
	}
	w.WriteMsg(resp)
}

func main() {
	addr := getenv("DNS_ADDR", ":53")
	loadZones()
	go watchZones()
	dns.HandleFunc(".", handle)
	go func() {
		s := &dns.Server{Addr: addr, Net: "udp", UDPBufferSize: 4096}
		log.Println("dns authoritative udp on", addr)
		if err := s.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	s := &dns.Server{Addr: addr, Net: "tcp"}
	log.Println("dns authoritative tcp on", addr)
	log.Fatal(s.ListenAndServe())
}
