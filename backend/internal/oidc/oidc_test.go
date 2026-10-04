package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthCodeURL(t *testing.T) {
	p := &Provider{AuthURL: "https://id.example/authorize"}
	u := p.AuthCodeURL("cid", "https://mail/cb", "st", "no")
	for _, want := range []string{"response_type=code", "client_id=cid", "state=st", "nonce=no", "scope=openid"} {
		if !contains(u, want) {
			t.Errorf("url 缺少 %q: %s", want, u)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestVerifyIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "k1"
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{"kty": "RSA", "kid": kid, "n": n, "e": e}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := &Provider{Issuer: "https://iss.example", JWKSURL: srv.URL + "/jwks"}
	claims := jwt.MapClaims{
		"iss": "https://iss.example", "aud": "cid", "sub": "u1",
		"email": "a@b.c", "name": "A", "nonce": "n1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	ui, err := p.VerifyIDToken(context.Background(), signed, "cid", "n1")
	if err != nil || ui.Email != "a@b.c" {
		t.Fatalf("应通过: %v %+v", err, ui)
	}
	if _, err := p.VerifyIDToken(context.Background(), signed, "cid", "bad"); err == nil {
		t.Fatal("nonce 不匹配应报错")
	}
	if _, err := p.VerifyIDToken(context.Background(), signed, "other", "n1"); err == nil {
		t.Fatal("aud 不匹配应报错")
	}
}
