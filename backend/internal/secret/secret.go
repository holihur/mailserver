// Package secret 提供基于 JWT_SECRET 派生密钥的对称加密，用于第三方服务商凭证落库。
// 切换 JWT_SECRET 会导致旧凭证无法解密，需要重新录入。
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"sync"
)

var (
	mu  sync.RWMutex
	key []byte
)

// SetKey 由 JWT_SECRET 派生 32 字节密钥（AES-256-GCM）。
func SetKey(s string) {
	h := sha256.Sum256([]byte(s))
	mu.Lock()
	key = h[:]
	mu.Unlock()
}

// Encrypt 返回 base64(nonce || ciphertext)。
func Encrypt(plain []byte) (string, error) {
	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return "", errors.New("secret: key 未初始化")
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}

// Decrypt 解密 Encrypt 的输出。
func Decrypt(enc string) ([]byte, error) {
	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return nil, errors.New("secret: key 未初始化")
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("secret: 密文损坏")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}
