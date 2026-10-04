package dnsserver

import (
	"crypto"
	"errors"
	"log"
	"os"
	"path/filepath"
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
