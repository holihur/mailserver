// Package emailauth 实现入站邮件的发件人认证：SPF 查询、DKIM 验签、DMARC 对齐。
// 纯标准库，DNS 通过 Lookup 注入，便于单元测试。用于给入站邮件打 Authentication-Results
// 标记，并（可选）按 DMARC 策略隔离/拒收。
package emailauth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
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

// ---- DMARC ----

// DMARC 查询发件域的 DMARC 策略（none/quarantine/reject）。present=false 表示无记录。
func DMARC(l Lookup, fromDomain string) (policy string, present bool) {
	fromDomain = strings.ToLower(strings.TrimSpace(fromDomain))
	if fromDomain == "" {
		return "", false
	}
	for _, d := range []string{fromDomain, OrgDomain(fromDomain)} {
		txts, err := l.TXT("_dmarc." + d)
		if err != nil {
			continue
		}
		for _, t := range txts {
			t = strings.TrimSpace(t)
			if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
				return dmarcPolicy(t), true
			}
		}
	}
	return "", false
}

func dmarcPolicy(rec string) string {
	for _, p := range strings.Split(rec, ";") {
		p = strings.TrimSpace(p)
		if i := strings.IndexByte(p, '='); i > 0 && strings.EqualFold(strings.TrimSpace(p[:i]), "p") {
			v := strings.ToLower(strings.TrimSpace(p[i+1:]))
			switch v {
			case "none", "quarantine", "reject":
				return v
			}
		}
	}
	return "none"
}

// OrgDomain 粗略取组织域（最后两级标签）。
func OrgDomain(host string) string {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// Aligned 判断 fromDomain 与认证域是否对齐（relaxed 时比较组织域）。
func Aligned(fromDomain, authDomain string, relaxed bool) bool {
	fromDomain = strings.ToLower(strings.TrimSpace(fromDomain))
	authDomain = strings.ToLower(strings.TrimSpace(authDomain))
	if fromDomain == "" || authDomain == "" {
		return false
	}
	if fromDomain == authDomain {
		return true
	}
	return relaxed && OrgDomain(fromDomain) == OrgDomain(authDomain)
}

// ---- DKIM 验签 ----

type hdr struct{ name, value string }

// VerifyDKIM 校验 raw 邮件中的 DKIM-Signature，返回 pass/fail/none。
// 支持 a=rsa-sha256 与 c=relaxed/simple | relaxed/relaxed。
func VerifyDKIM(l Lookup, raw []byte) string {
	headBlock, body := splitMessage(string(raw))
	headers := parseHeaders(headBlock)
	var sigs []string
	for _, h := range headers {
		if strings.EqualFold(h.name, "DKIM-Signature") {
			sigs = append(sigs, h.value)
		}
	}
	if len(sigs) == 0 {
		return "none"
	}
	for _, sig := range sigs {
		if verifyOne(l, headers, body, sig) {
			return "pass"
		}
	}
	return "fail"
}

func verifyOne(l Lookup, headers []hdr, body, sig string) bool {
	tags := parseTags(sig)
	if tags["v"] != "1" || tags["a"] != "rsa-sha256" {
		return false
	}
	d, s := tags["d"], tags["s"]
	if d == "" || s == "" || tags["b"] == "" || tags["bh"] == "" {
		return false
	}
	hc, bc := "relaxed", "simple"
	if c := tags["c"]; c != "" {
		parts := strings.SplitN(strings.TrimSpace(c), "/", 2)
		if len(parts) == 2 {
			hc, bc = strings.ToLower(parts[0]), strings.ToLower(parts[1])
		} else {
			hc = strings.ToLower(parts[0])
			bc = hc
		}
	}
	// 体哈希
	var bodyCanon string
	switch bc {
	case "relaxed":
		bodyCanon = canonBodyRelaxed(body)
	default:
		bodyCanon = canonBodySimple(body)
	}
	sum := sha256.Sum256([]byte(bodyCanon))
	if !strings.EqualFold(tags["bh"], base64.StdEncoding.EncodeToString(sum[:])) {
		return false
	}
	// 已签名头
	var b strings.Builder
	used := map[string]int{}
	for _, name := range strings.Split(tags["h"], ":") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		idx := findHeaderFromBottom(headers, name, used[name])
		if idx < 0 {
			return false
		}
		used[name]++
		b.WriteString(name + ":" + canonHeader(hc, headers[idx].value) + "\r\n")
	}
	b.WriteString("dkim-signature:" + canonHeader(hc, stripTag(sig, "b")) + "\r\n")
	h := sha256.Sum256([]byte(b.String()))

	txts, err := l.TXT(s + "._domainkey." + d)
	if err != nil {
		return false
	}
	pub := findPubKey(txts)
	if pub == "" {
		return false
	}
	key, err := parseRSAPub(pub)
	if err != nil {
		return false
	}
	sigBytes, err := base64.StdEncoding.DecodeString(tags["b"])
	if err != nil {
		return false
	}
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sigBytes) == nil
}

func parseRSAPub(b64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	k, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not rsa")
	}
	return k, nil
}

// DKIMDomain 返回第一封 DKIM-Signature 的 d= 域。
func DKIMDomain(raw []byte) string {
	headBlock, _ := splitMessage(string(raw))
	for _, h := range parseHeaders(headBlock) {
		if strings.EqualFold(h.name, "DKIM-Signature") {
			if d := parseTags(h.value)["d"]; d != "" {
				return strings.ToLower(d)
			}
		}
	}
	return ""
}

// Result 汇总一封入站邮件的认证结果。
type Result struct {
	SPF    string // pass/fail/softfail/neutral/none/temperror/permerror
	DKIM   string // pass/fail/none
	DMARC  string // pass/fail/none
	Policy string // none/quarantine/reject（有 DMARC 记录时）
}

// Evaluate 计算 SPF/DKIM/DMARC 结果。envelopeFrom 为 MAIL FROM 地址。
func Evaluate(l Lookup, senderIP net.IP, envelopeFrom string, raw []byte) Result {
	envDom := domainOf(envelopeFrom)
	fromDom := FromDomain(raw)
	if fromDom == "" {
		fromDom = envDom
	}
	spf := SPF(l, senderIP, envDom)
	spfAligned := spf == SPFPass && Aligned(fromDom, envDom, true)
	dkim := VerifyDKIM(l, raw)
	dkimDom := ""
	if dkim == "pass" {
		dkimDom = DKIMDomain(raw)
	}
	dkimAligned := dkim == "pass" && Aligned(fromDom, dkimDom, true)
	policy, present := DMARC(l, fromDom)
	dmarc := "none"
	if present {
		if spfAligned || dkimAligned {
			dmarc = "pass"
		} else {
			dmarc = "fail"
		}
	} else {
		policy = ""
	}
	return Result{SPF: spf, DKIM: dkim, DMARC: dmarc, Policy: policy}
}

// FromDomain 从 RFC5322 From 头提取域（用于 DMARC）。
func FromDomain(raw []byte) string {
	headBlock, _ := splitMessage(string(raw))
	for _, h := range parseHeaders(headBlock) {
		if strings.EqualFold(h.name, "From") {
			if a, err := mail.ParseAddress(strings.TrimSpace(h.value)); err == nil {
				return domainOf(a.Address)
			}
		}
	}
	return ""
}

func domainOf(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.TrimSpace(addr[i+1:])
	}
	return ""
}

// String 返回可存入 Mail.AuthResults 的简短描述。
func (r Result) String() string {
	return fmt.Sprintf("spf=%s; dkim=%s; dmarc=%s", r.SPF, r.DKIM, r.DMARC)
}

func findPubKey(txts []string) string {
	for _, t := range txts {
		for _, p := range strings.Split(t, ";") {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(strings.ToLower(p), "p=") {
				return strings.TrimSpace(p[2:])
			}
		}
	}
	return ""
}

func splitMessage(s string) (head, body string) {
	if i := strings.Index(s, "\r\n\r\n"); i >= 0 {
		return s[:i], s[i+4:]
	}
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[:i], s[i+2:]
	}
	return s, ""
}

func parseHeaders(block string) []hdr {
	var out []hdr
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1].value += "\r\n" + line
			continue
		}
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		out = append(out, hdr{name: line[:i], value: line[i+1:]})
	}
	return out
}

func findHeaderFromBottom(headers []hdr, name string, nth int) int {
	count := 0
	for i := len(headers) - 1; i >= 0; i-- {
		if strings.EqualFold(headers[i].name, name) {
			if count == nth {
				return i
			}
			count++
		}
	}
	return -1
}

func parseTags(s string) map[string]string {
	m := map[string]string{}
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		i := strings.IndexByte(p, '=')
		if i < 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(p[:i]))
		v := strings.Join(strings.Fields(p[i+1:]), "")
		m[k] = v
	}
	return m
}

// stripTag 返回把 key 的值清空后的头（用于 b=）。
func stripTag(sig, key string) string {
	var parts []string
	for _, p := range strings.Split(sig, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(p), key+"=") {
			parts = append(parts, key+"=")
		} else {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "; ")
}

func canonHeader(canon, v string) string {
	v = strings.TrimPrefix(v, " ")
	if canon == "relaxed" {
		return relax(v)
	}
	return strings.TrimRight(v, "\r\n")
}

func relax(v string) string {
	v = strings.ReplaceAll(v, "\r\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	return strings.Join(strings.Fields(v), " ")
}

func canonBodySimple(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "\r\n"
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func canonBodyRelaxed(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}
