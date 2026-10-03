package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSignAndParse(t *testing.T) {
	SetSecret("unit-test-secret")
	tok, err := Sign(42, "a@b.c")
	if err != nil || tok == "" {
		t.Fatalf("sign: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	uid, err := UserID(req)
	if err != nil || uid != 42 {
		t.Fatalf("UserID=%d err=%v", uid, err)
	}
}

func TestUserIDErrors(t *testing.T) {
	SetSecret("unit-test-secret")

	// 无 Authorization
	if _, err := UserID(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Fatal("expected error without token")
	}
	// 非 Bearer
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Token abc")
	if _, err := UserID(req); err == nil {
		t.Fatal("expected error for non-bearer")
	}
	// 非法 token
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer not-a-jwt")
	if _, err := UserID(req2); err == nil {
		t.Fatal("expected error for bad token")
	}
	// 另一个密钥签的 token 应校验失败
	SetSecret("other-secret")
	tok, _ := Sign(1, "x@y.z")
	SetSecret("unit-test-secret")
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	if _, err := UserID(req3); err == nil {
		t.Fatal("expected signature mismatch")
	}
}
