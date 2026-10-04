// Package dnsserver 是本项目内置的权威 DNS 服务器（原 dns/main.go），
// 现在与 API 编译进同一个二进制，避免单独部署 nsd。
// 只应答本机托管域名的 A/AAAA/MX/TXT/NS/SOA/CNAME/SRV/CAA；非托管域 REFUSED（不递归）。
package dnsserver

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

type Server struct {
	ZonesPath string
	NSHost    string
	mu        sync.RWMutex
	zones     []Zone
}

// New 创建 DNS 服务器；zonesPath 为 backend 导出的 zones.json。
func New(zonesPath, nsHost string) *Server {
	return &Server{ZonesPath: zonesPath, NSHost: nsHost}
}

// Start 加载 zones 并启动 UDP+TCP 监听（阻塞在 TCP 上，建议 go 调用）。
func (s *Server) Start(addr string) {
	s.load()
	go s.watch()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	go func() {
		srv := &dns.Server{Addr: addr, Net: "udp", UDPSize: 4096, Handler: mux}
		if err := srv.ListenAndServe(); err != nil {
			log.Println("dns udp listen:", err)
		}
	}()
	srv := &dns.Server{Addr: addr, Net: "tcp", Handler: mux}
	if err := srv.ListenAndServe(); err != nil {
		log.Println("dns tcp listen:", err)
	}
}

func (s *Server) load() {
	b, err := os.ReadFile(s.ZonesPath)
	if err != nil {
		log.Println("dns: 无 zones 文件:", s.ZonesPath, "（等 backend 生成）")
		return
	}
	var v struct {
		Zones []Zone `json:"zones"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		log.Println("dns: zones 解析失败:", err)
		return
	}
	s.mu.Lock()
	s.zones = v.Zones
	s.mu.Unlock()
	log.Printf("dns: 加载 %d 个 zone", len(v.Zones))
}

func (s *Server) watch() {
	var last time.Time
	if fi, err := os.Stat(s.ZonesPath); err == nil {
		last = fi.ModTime()
	}
	for range time.Tick(10 * time.Second) {
		fi, err := os.Stat(s.ZonesPath)
		if err != nil {
			continue
		}
		if fi.ModTime().After(last) {
			last = fi.ModTime()
			s.load()
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

func (s *Server) findZone(q string) *Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q = strings.ToLower(q)
	var best *Zone
	for i := range s.zones {
		suf := strings.ToLower(s.zones[i].Domain) + "."
		if q == suf || strings.HasSuffix(q, "."+suf) {
			if best == nil || len(s.zones[i].Domain) > len(best.Domain) {
				b := s.zones[i]
				best = &b
			}
		}
	}
	return best
}

func (s *Server) soaRR(zone string) dns.RR {
	out := &dns.SOA{
		Hdr:     dns.RR_Header{Name: zone + ".", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      "ns1." + zone + ".",
		Mbox:    "hostmaster." + zone + ".",
		Serial:  1,
		Refresh: 7200, Retry: 3600, Expire: 1209600, Minttl: 300,
	}
	if s.NSHost != "" {
		out.Ns = s.NSHost
	}
	return out
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
			n := strings.Fields(r.Value)
			if len(n) == 4 {
				pr, we, po := atoi(n[0]), atoi(n[1]), atoi(n[2])
				tgt := n[3]
				if !strings.HasSuffix(tgt, ".") {
					tgt = fqdn(tgt, z.Domain)
				}
				out = append(out, &dns.SRV{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: ttl}, Priority: uint16(pr), Weight: uint16(we), Port: uint16(po), Target: tgt})
			}
		case qtype == dns.TypeCAA && t == "CAA":
			flag, tag, val := parseCAA(r.Value)
			out = append(out, &dns.CAA{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: ttl}, Flag: flag, Tag: tag, Value: val})
		}
	}
	return out
}

// TXT 单段 ≤255 字节才合法（如 2048 位 DKIM 公钥必须切分）
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

func (s *Server) handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	resp.RecursionAvailable = false
	if len(req.Question) == 0 {
		_ = w.WriteMsg(resp)
		return
	}
	q := req.Question[0]
	z := s.findZone(q.Name)
	if z == nil {
		resp.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(resp)
		return
	}
	switch q.Qtype {
	case dns.TypeSOA:
		resp.Answer = s.signAnswers(z.Domain, []dns.RR{s.soaRR(z.Domain)})
	case dns.TypeDNSKEY:
		resp.Answer = s.DNSKEYs(z.Domain)
	case dns.TypeAXFR, dns.TypeIXFR:
		resp.Rcode = dns.RcodeRefused
	default:
		var ans []dns.RR
		for _, rr := range toRRs(z, q.Qtype) {
			if strings.EqualFold(rr.Header().Name, q.Name) {
				ans = append(ans, rr)
			}
		}
		if len(ans) == 0 {
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
			resp.Ns = []dns.RR{s.soaRR(z.Domain)}
		} else {
			resp.Answer = s.signAnswers(z.Domain, ans)
		}
	}
	_ = w.WriteMsg(resp)
}
