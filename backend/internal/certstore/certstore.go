// Package certstore 保存当前 TLS 证书并支持运行时热替换（管理后台上传或 ACME 自动签发后立即生效）。
package certstore

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Meta struct {
	Exists    bool      `json:"exists"`
	Domains   []string  `json:"domains"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Source    string    `json:"source"` // manual | acme | env | file
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	dir  string
	mu   sync.RWMutex
	cert *tls.Certificate
	meta Meta
}

func New(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) certPath() string { return filepath.Join(s.dir, "cert.pem") }
func (s *Store) keyPath() string  { return filepath.Join(s.dir, "key.pem") }

// Load 从磁盘恢复证书（若存在）。
func (s *Store) Load() error {
	certPEM, err := os.ReadFile(s.certPath())
	if err != nil {
		return nil // 没有证书不算错误
	}
	keyPEM, err := os.ReadFile(s.keyPath())
	if err != nil {
		return err
	}
	return s.Set(certPEM, keyPEM, "file")
}

// Set 解析并保存证书（内存立即生效 + 落盘）。
func (s *Store) Set(certPEM, keyPEM []byte, source string) error {
	ce, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return err
	}
	if len(ce.Certificate) == 0 {
		return errors.New("证书为空")
	}
	leaf, err := x509.ParseCertificate(ce.Certificate[0])
	if err != nil {
		return err
	}
	ce.Leaf = leaf
	meta := Meta{
		Exists:    true,
		Issuer:    leaf.Issuer.CommonName,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		Source:    source,
		UpdatedAt: time.Now(),
	}
	seen := map[string]bool{}
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			meta.Domains = append(meta.Domains, d)
		}
	}
	if leaf.Subject.CommonName != "" {
		add(leaf.Subject.CommonName)
	}
	for _, d := range leaf.DNSNames {
		add(d)
	}

	s.mu.Lock()
	s.cert = &ce
	s.meta = meta
	s.mu.Unlock()

	if s.dir != "" {
		if err := os.MkdirAll(s.dir, 0700); err == nil {
			_ = os.WriteFile(s.certPath(), certPEM, 0644)
			_ = os.WriteFile(s.keyPath(), keyPEM, 0600)
		}
	}
	return nil
}

// Clear 删除证书。
func (s *Store) Clear() error {
	s.mu.Lock()
	s.cert = nil
	s.meta = Meta{}
	s.mu.Unlock()
	_ = os.Remove(s.certPath())
	_ = os.Remove(s.keyPath())
	return nil
}

func (s *Store) Meta() Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

// GetCertificate 供 tls.Config 动态取证书。
func (s *Store) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cert == nil {
		return nil, errors.New("尚未配置 TLS 证书")
	}
	return s.cert, nil
}

// TLSConfig 返回 mail 服务共用的动态 TLS 配置。
func (s *Store) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}
