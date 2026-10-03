package dkim

// DKIM 自签名（relaxed/simple + rsa-sha256），纯标准库，无新依赖。
// 私钥放服务器（如 /data/dkim.pem），公钥经 API 自动写入自家 DNS 的 selector._domainkey TXT。
// 2048 位公钥 p= 约 400 字符，dns/ 服务会自动切成多段 TXT，不用操心 255 限制。

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Signer struct {
	Domain   string
	Selector string
	Key      *rsa.PrivateKey
}

func Load(domain, selector, keyPath string) (*Signer, error) {
	if domain == "" || keyPath == "" {
		return nil, errors.New("dkim: domain/keyPath required")
	}
	if selector == "" {
		selector = "dkim"
	}
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errors.New("dkim: bad pem")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		var ok bool
		key, ok = k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("dkim: not rsa key")
		}
	} else {
		return nil, errors.New("dkim: parse key failed")
	}
	return &Signer{Domain: strings.ToLower(domain), Selector: selector, Key: key}, nil
}

// Match: 发件人域名与签名域一致才签（否则 d= 对不上，签了也无效）
func (s *Signer) Match(from string) bool {
	if s == nil {
		return false
	}
	at := strings.LastIndex(from, "@")
	if at < 0 {
		return false
	}
	return strings.EqualFold(strings.Trim(from[at+1:], ". "), s.Domain)
}

// 公钥 TXT 值（写入 selector._domainkey 的内容）
func (s *Signer) TXT() string {
	der, err := x509.MarshalPKIXPublicKey(&s.Key.PublicKey)
	if err != nil {
		return ""
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)
}

// 对已定稿的头 + body 签名，返回完整 "DKIM-Signature: ..." 行（失败返回 ""）
func (s *Signer) Sign(headers [][2]string, body string) string {
	if s == nil || s.Key == nil {
		return ""
	}
	var names []string
	var b strings.Builder
	for _, h := range headers {
		n := strings.ToLower(strings.TrimSpace(h[0]))
		names = append(names, n)
		b.WriteString(n + ":" + relax(h[1]) + "\r\n")
	}
	bh := sha256.Sum256([]byte(canonBody(body)))
	dk := fmt.Sprintf("v=1; a=rsa-sha256; c=relaxed/simple; d=%s; s=%s; h=%s; bh=%s; b=",
		s.Domain, s.Selector, strings.Join(names, " : "),
		base64.StdEncoding.EncodeToString(bh[:]))
	b.WriteString("dkim-signature:" + relax(dk) + "\r\n")
	h := sha256.Sum256([]byte(b.String()))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.Key, crypto.SHA256, h[:])
	if err != nil {
		return ""
	}
	return "DKIM-Signature: " + dk + base64.StdEncoding.EncodeToString(sig)
}

// relaxed 头值：hdr 换行转空格，多余空白压缩，首尾去空
func relax(v string) string {
	v = strings.ReplaceAll(v, "\r\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	return strings.Join(strings.Fields(v), " ")
}

// simple 体：统一 CRLF，去掉末尾空行；空体为单个 CRLF
func canonBody(body string) string {
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
