// Package emailauth 实现入站邮件的发件人认证：SPF 查询、DKIM 验签、DMARC 对齐。
// 纯标准库，DNS 通过 Lookup 注入，便于单元测试。用于给入站邮件打 Authentication-Results
// 标记，并（可选）按 DMARC 策略隔离/拒收。
package emailauth

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// Lookup 抽象 DNS 查询，测试可注入假实现。
type Lookup struct {
	TXT func(domain string) ([]string, error)
	IP  func(host string) ([]net.IP, error)
	MX  func(domain string) ([]*net.MX, error)
}

// NetLookup 使用真实 DNS。
var NetLookup = Lookup{TXT: net.LookupTXT, IP: net.LookupIP, MX: net.LookupMX}

// ---- SPF ----

const (
	SPFPass      = "pass"
	SPFFail      = "fail"
	SPFSoftfail  = "softfail"
	SPFNeutral   = "neutral"
	SPFNone      = "none"
	SPFTempError = "temperror"
	SPFPermError = "permerror"
)

// SPF 返回对 senderIP 的 SPF 校验结果。
func SPF(l Lookup, senderIP net.IP, domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || senderIP == nil {
		return SPFNone
	}
	rec, err := findSPF(l, domain)
	if err != nil {
		if errors.Is(err, errMultiple) {
			return SPFPermError
		}
		return SPFNone // 无记录 / DNS 不可达均按无记录处理（保守，不误判）
	}
	fields := strings.Fields(rec)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "v=spf1") {
		return SPFPermError
	}
	redirect := ""
	for _, mech := range fields[1:] {
		if strings.HasPrefix(strings.ToLower(mech), "redirect=") {
			redirect = mech[len("redirect="):]
			continue
		}
		res, matched, mErr := matchSPF(l, senderIP, domain, mech, 0)
		if mErr != nil {
			return SPFTempError
		}
		if matched {
			return res
		}
	}
	if redirect != "" {
		r := SPF(l, senderIP, redirect)
		if r == SPFNone {
			return SPFPermError
		}
		return r
	}
	return SPFNeutral
}

var errNoRecord = errors.New("no spf record")
var errMultiple = errors.New("multiple spf records")

func findSPF(l Lookup, domain string) (string, error) {
	txts, err := l.TXT(domain)
	if err != nil {
		return "", err
	}
	found := ""
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			if found != "" {
				return "", errMultiple
			}
			found = strings.TrimSpace(t)
		}
	}
	if found == "" {
		return "", errNoRecord
	}
	return found, nil
}

func matchSPF(l Lookup, ip net.IP, domain, mech string, depth int) (string, bool, error) {
	if depth > 10 {
		return "", false, fmt.Errorf("spf too deep")
	}
	if mech == "" {
		return "", false, nil
	}
	qual := SPFPass
	switch mech[0] {
	case '+':
		mech = mech[1:]
	case '-':
		qual, mech = SPFFail, mech[1:]
	case '~':
		qual, mech = SPFSoftfail, mech[1:]
	case '?':
		qual, mech = SPFNeutral, mech[1:]
	}
	name, arg := mech, ""
	if i := strings.IndexByte(mech, ':'); i >= 0 {
		name, arg = mech[:i], mech[i+1:]
	}
	switch strings.ToLower(name) {
	case "all":
		return qual, true, nil
	case "ip4", "ip6":
		if n, ok := parseIPNet(arg); ok && n.Contains(ip) {
			return qual, true, nil
		}
	case "a":
		host := arg
		if host == "" {
			host = domain
		}
		ips, err := l.IP(host)
		if err != nil {
			return "", false, err
		}
		if containsIP(ips, ip) {
			return qual, true, nil
		}
	case "mx":
		host := arg
		if host == "" {
			host = domain
		}
		mxs, err := l.MX(host)
		if err != nil {
			return "", false, err
		}
		for _, mx := range mxs {
			ips, err := l.IP(mx.Host)
			if err != nil {
				continue
			}
			if containsIP(ips, ip) {
				return qual, true, nil
			}
		}
	case "include":
		switch SPF(l, ip, arg) {
		case SPFPass:
			return qual, true, nil
		case SPFTempError, SPFPermError:
			return "", false, fmt.Errorf("include %s", arg)
		}
	case "exists":
		if ips, err := l.IP(arg); err == nil && len(ips) > 0 {
			return qual, true, nil
		}
	}
	return "", false, nil
}

func parseIPNet(s string) (*net.IPNet, bool) {
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n, true
	}
	if ip := net.ParseIP(s); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, true
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, true
	}
	return nil, false
}

func containsIP(ips []net.IP, ip net.IP) bool {
	for _, x := range ips {
		if x.Equal(ip) {
			return true
		}
	}
	return false
}
