// Package dane 实现出站 SMTP 的 DANE（RFC 7672）：查询对方 MX 的 TLSA 记录，
// 仅在 DNSSEC 验证通过（解析器返回 AD）时强制证书匹配；不匹配则拒绝明文/降级投递。
package dane

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Enabled 由 main 从配置注入；关闭时不做任何 DANE 校验。
var Enabled bool

// Resolver 用于 DNSSEC 验证的递归解析器 host:port；空则读 /etc/resolv.conf。
// 建议使用会校验 DNSSEC 并置 AD 位的解析器（如 127.0.0.1 上的 unbound）。
var Resolver string

// Record 一条 TLSA 记录。
type Record struct {
	Usage        uint8
	Selector     uint8
	MatchingType uint8
	Cert         []byte
}

// Lookup 查询 _<port>._tcp.<host> 的 TLSA 记录。
// ok 表示解析器标记了 DNSSEC 验证通过（AD=1）；只有 ok 且记录非空才应强制匹配。
func Lookup(host string, port int) (recs []Record, ok bool, err error) {
	name := fmt.Sprintf("_%d._tcp.%s", port, dns.Fqdn(host))
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeTLSA)
	m.SetEdns0(4096, true) // 请求 DNSSEC（DO 位）

	c := &dns.Client{Timeout: 5 * time.Second}
	var lastErr error
	for _, srv := range servers() {
		resp, _, e := c.Exchange(m, srv)
		if e != nil {
			lastErr = e
			continue
		}
		for _, rr := range resp.Answer {
			if t, isTLSA := rr.(*dns.TLSA); isTLSA {
				cert, e := hex.DecodeString(t.Certificate)
				if e != nil {
					continue
				}
				recs = append(recs, Record{Usage: t.Usage, Selector: t.Selector, MatchingType: t.MatchingType, Cert: cert})
			}
		}
		return recs, resp.AuthenticatedData, nil
	}
	if lastErr == nil {
		lastErr = errors.New("dane: 无可用 DNS 解析器")
	}
	return nil, false, lastErr
}

// TLSConfig 为指定 MX 生成带 DANE 校验的 TLS 配置。
// 含 DANE-EE（usage=3）时跳过系统 PKIX 校验（自签也有效，由 TLSA 锚定）。
func TLSConfig(host string, recs []Record) *tls.Config {
	skipPKIX := false
	for _, r := range recs {
		if r.Usage == 3 {
			skipPKIX = true
		}
	}
	return &tls.Config{
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skipPKIX, //nolint:gosec // DANE-EE 由 TLSA 锚定，非裸奔
		VerifyConnection: func(cs tls.ConnectionState) error {
			return Verify(recs, cs.PeerCertificates)
		},
	}
}

// Verify 按 RFC 6698 校验对端证书链是否匹配任一 TLSA 记录。
func Verify(recs []Record, certs []*x509.Certificate) error {
	if len(certs) == 0 {
		return errors.New("dane: 无对端证书")
	}
	for _, r := range recs {
		var candidates [][]byte
		switch r.Usage {
		case 3, 1: // DANE-EE / PKIX-EE：匹配终端叶证书
			candidates = append(candidates, material(certs[0], r.Selector))
		case 2, 0: // DANE-TA / PKIX-TA：匹配链上任意证书
			for _, c := range certs {
				candidates = append(candidates, material(c, r.Selector))
			}
		default:
			continue
		}
		for _, cand := range candidates {
			if matches(cand, r) {
				return nil
			}
		}
	}
	return errors.New("dane: 证书与 TLSA 记录不匹配")
}

func material(cert *x509.Certificate, selector uint8) []byte {
	if selector == 1 { // SPKI
		return cert.RawSubjectPublicKeyInfo
	}
	return cert.Raw // 整个证书
}

func matches(candidate []byte, r Record) bool {
	switch r.MatchingType {
	case 0:
		return bytes.Equal(candidate, r.Cert)
	case 1:
		sum := sha256.Sum256(candidate)
		return bytes.Equal(sum[:], r.Cert)
	case 2:
		sum := sha512.Sum512(candidate)
		return bytes.Equal(sum[:], r.Cert)
	}
	return false
}

func servers() []string {
	if strings.TrimSpace(Resolver) != "" {
		return []string{Resolver}
	}
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || len(cfg.Servers) == 0 {
		return []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	out := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		out = append(out, net.JoinHostPort(s, cfg.Port))
	}
	return out
}
