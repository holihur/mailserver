package emailauth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

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
	// 已签名头：simple 用原文（原大小写与折行），relaxed 用小写名 + 折叠空白
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
		if hc == "simple" {
			b.WriteString(headers[idx].name + ":" + headers[idx].value + "\r\n")
		} else {
			b.WriteString(name + ":" + relax(headers[idx].value) + "\r\n")
		}
	}
	if hc == "simple" {
		b.WriteString("DKIM-Signature:" + stripTagB(sig) + "\r\n")
	} else {
		b.WriteString("dkim-signature:" + relax(stripTagB(sig)) + "\r\n")
	}
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

// stripTagB 将 DKIM-Signature 中 b= 的值置空，保留其余原文（供 canonicalization）。
// 用正则而非解析重组，避免破坏原有折行/空白（simple 规范化依赖原文）。
var bTagRe = regexp.MustCompile(`(?i)(b\s*=)[^;]*`)

func stripTagB(sig string) string {
	return bTagRe.ReplaceAllString(sig, "${1}")
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
		// RFC 6376 3.4.4：行内连续空白压成单个空格，行尾空白删除
		lines[i] = collapseWSP(lines[i])
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

// collapseWSP 把一行内连续的空格/制表符压成单个空格，并去掉行首/行尾空白。
func collapseWSP(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pending := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			pending = true
			continue
		}
		if pending {
			b.WriteByte(' ')
			pending = false
		}
		b.WriteByte(c)
	}
	return b.String()
}
