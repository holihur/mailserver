// Package totp 实现 RFC 6238 TOTP（基于 HMAC-SHA1，6 位，30 秒），仅用标准库。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成 160bit 随机密钥（base32，无 padding）。
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	s = strings.TrimRight(s, "=")
	return b32.DecodeString(s)
}

// Code 计算指定时间的 6 位动态码。
func Code(secret string, t time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(t.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", bin%1000000), nil
}

// Verify 校验动态码，容忍前后各 1 个时间窗（±30s）。
func Verify(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	now := time.Now()
	for _, d := range []int{-1, 0, 1} {
		c, err := Code(secret, now.Add(time.Duration(d)*30*time.Second))
		if err == nil && hmac.Equal([]byte(c), []byte(code)) {
			return true
		}
	}
	return false
}

// ProvisioningURL 生成 otpauth:// 链接，供验证器 App 扫码/手动录入。
func ProvisioningURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return "otpauth://totp/" + label + "?" + v.Encode()
}
