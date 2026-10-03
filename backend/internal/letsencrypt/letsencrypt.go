// Package letsencrypt 使用 ACME DNS-01 质询向 Let's Encrypt 申请证书。
// DNS 记录通过 provider（阿里云 / Cloudflare）自动写入 _acme-challenge，无需开放 80/443。
package letsencrypt

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/acme"
)

const (
	LetsEncryptURL = "https://acme-v02.api.letsencrypt.org/directory"
	StagingURL     = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Solver 负责在 DNS 服务商处增删质询记录。
type Solver interface {
	Present(ctx context.Context, recordName, value string) error
	CleanUp(ctx context.Context, recordName, value string) error
	Wait(ctx context.Context, recordName, value string) error
}

type Issuer struct {
	client *acme.Client
	dir    string
}

// NewIssuer 加载或创建 ACME 账号密钥（ECDSA P-256）。
func NewIssuer(dir string, staging bool) (*Issuer, error) {
	if dir != "" {
		_ = os.MkdirAll(dir, 0700)
	}
	key, err := loadOrCreateAccountKey(filepath.Join(dir, "acme-account.pem"))
	if err != nil {
		return nil, err
	}
	url := LetsEncryptURL
	if staging {
		url = StagingURL
	}
	return &Issuer{client: &acme.Client{Key: key, DirectoryURL: url}, dir: dir}, nil
}

func (i *Issuer) Staging() bool { return i.client.DirectoryURL == StagingURL }

// Issue 申请证书，返回 cert/key 的 PEM。域名通常为 mail.example.com（也可带主域）。
func (i *Issuer) Issue(ctx context.Context, domains []string, email string, solver Solver) (certPEM, keyPEM []byte, err error) {
	if len(domains) == 0 {
		return nil, nil, errors.New("域名不能为空")
	}
	acct := &acme.Account{}
	if email != "" {
		acct.Contact = []string{"mailto:" + email}
	}
	if _, err := i.client.Register(ctx, acct, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, nil, fmt.Errorf("注册 ACME 账号失败: %w", err)
	}

	order, err := i.client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return nil, nil, fmt.Errorf("创建证书订单失败: %w", err)
	}

	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	for _, authzURL := range order.AuthzURLs {
		authz, err := i.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, nil, fmt.Errorf("获取授权失败: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "dns-01" {
				chal = c
				break
			}
		}
		if chal == nil {
			return nil, nil, fmt.Errorf("%s 未提供 dns-01 质询", authz.Identifier.Value)
		}
		value, err := i.client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, nil, err
		}
		recordName := "_acme-challenge." + authz.Identifier.Value
		if err := solver.Present(ctx, recordName, value); err != nil {
			return nil, nil, fmt.Errorf("写入 DNS 质询失败: %w", err)
		}
		name, val := recordName, value
		cleanups = append(cleanups, func() {
			_ = solver.CleanUp(context.Background(), name, val)
		})
		if err := solver.Wait(ctx, recordName, value); err != nil {
			return nil, nil, fmt.Errorf("等待 DNS 生效失败: %w", err)
		}
		if _, err := i.client.Accept(ctx, chal); err != nil {
			return nil, nil, fmt.Errorf("确认质询失败: %w", err)
		}
		if _, err := i.client.WaitAuthorization(ctx, authzURL); err != nil {
			return nil, nil, fmt.Errorf("质询未通过: %w", err)
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}, key)
	if err != nil {
		return nil, nil, err
	}
	der, _, err := i.client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, nil, fmt.Errorf("签发证书失败: %w", err)
	}
	var buf bytes.Buffer
	for _, b := range der {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: b})
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return buf.Bytes(), keyPEM, nil
}

func loadOrCreateAccountKey(path string) (*ecdsa.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		if blk, _ := pem.Decode(b); blk != nil {
			if k, err := x509.ParseECPrivateKey(blk.Bytes); err == nil {
				return k, nil
			}
			if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
				if ec, ok := k.(*ecdsa.PrivateKey); ok {
					return ec, nil
				}
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if path != "" {
		b := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
		_ = os.WriteFile(path, b, 0600)
	}
	return key, nil
}
