package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSignAndParse(t *testing.T) {
	SetSecret("unit-test-secret")
	tok, err := Sign(42, "a@b.c", 0)
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
	tok, _ := Sign(1, "x@y.z", 0)
	SetSecret("unit-test-secret")
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	if _, err := UserID(req3); err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func TestTokenTypes(t *testing.T) {
	SetSecret("unit-test-secret")

	// TOTP 挑战令牌不能当作 access 令牌使用
	ch, err := SignTOTPChallenge(7, "u@x.y")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+ch)
	if _, err := UserID(req); err == nil {
		t.Fatal("挑战令牌不应能通过 UserID")
	}
	if uid, err := TOTPChallengeUserID(ch); err != nil || uid != 7 {
		t.Fatalf("挑战令牌解析失败: uid=%d err=%v", uid, err)
	}

	// access 令牌不能当挑战令牌
	acc, _ := Sign(7, "u@x.y", 0)
	if _, err := TOTPChallengeUserID(acc); err == nil {
		t.Fatal("access 令牌不应通过 TOTPChallengeUserID")
	}
}

func TestTokenVersion(t *testing.T) {
	SetSecret("unit-test-secret")
	tok, _ := Sign(5, "v@x.y", 3)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	uid, ver, jti, err := Access(req)
	if err != nil || uid != 5 || ver != 3 || jti != "" {
		t.Fatalf("Access=%d,%d,%q,err=%v", uid, ver, jti, err)
	}
}

func TestClaimUIDZero(t *testing.T) {
	SetSecret("unit-test-secret")
	tok, _ := Sign(0, "x@y.z", 0)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if _, err := UserID(req); err == nil {
		t.Fatal("uid=0 应报错")
	}
	if _, err := TOTPChallengeUserID("bad-token"); err == nil {
		t.Fatal("坏挑战令牌应报错")
	}
}
