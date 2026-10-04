// Package srs 实现 Sender Rewriting Scheme（SRS0）子集：转发时重写信封发件人，
// 保证经中继转发后 SPF 对齐，且可还原原始地址。
package srs

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"strings"
	"time"
)

const prefix = "SRS0"

func hash4(secret []byte, tt, domain, local string) string {
	mac := hmac.New(sha1.New, secret)
	mac.Write([]byte(tt + "|" + domain + "|" + local))
	sum := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil))
	return sum[:4]
}

// Encode 把 addr 重写为 SRS0=HHHH=TT=domain=local@aliasDomain。
func Encode(secret []byte, addr, aliasDomain string) string {
	addr = strings.TrimSpace(addr)
	at := strings.LastIndex(addr, "@")
	if at <= 0 || at == len(addr)-1 || len(secret) == 0 {
		return addr
	}
	local, domain := addr[:at], addr[at+1:]
	tt := time.Now().Format("0102")
	return prefix + "=" + hash4(secret, tt, domain, local) + "=" + tt + "=" + domain + "=" + local + "@" + aliasDomain
}

// Decode 还原 SRS0 地址；非 SRS 或校验失败返回 ok=false。
func Decode(secret []byte, addr string) (string, bool) {
	at := strings.LastIndex(addr, "@")
	if at <= 0 {
		return "", false
	}
	parts := strings.SplitN(addr[:at], "=", 5)
	if len(parts) != 5 || !strings.EqualFold(parts[0], prefix) {
		return "", false
	}
	hh, tt, domain, origLocal := parts[1], parts[2], parts[3], parts[4]
	if len(secret) == 0 || !hmac.Equal([]byte(hh), []byte(hash4(secret, tt, domain, origLocal))) {
		return "", false
	}
	return origLocal + "@" + domain, true
}
