package totp

import (
	"strings"
	"testing"
	"time"
)

func TestCodeVerify(t *testing.T) {
	sec, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := Code(sec, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code=%q", code)
	}
	if !Verify(sec, code) {
		t.Fatal("当前动态码应通过")
	}
	if code != "000000" && Verify(sec, "000000") {
		t.Fatal("错误动态码不应通过")
	}
	if Verify(sec, "12345") {
		t.Fatal("长度错误不应通过")
	}
}

func TestDeterministic(t *testing.T) {
	sec := "JBSWY3DPEHPK3PXP"
	t0 := time.Unix(0, 0)
	c1, _ := Code(sec, t0)
	c2, _ := Code(sec, t0)
	if c1 != c2 || len(c1) != 6 {
		t.Fatalf("同输入应稳定: %q %q", c1, c2)
	}
	// 不同时间窗通常不同
	c3, _ := Code(sec, t0.Add(30*time.Second))
	if c1 == c3 {
		t.Fatalf("相邻时间窗不应相同: %q", c1)
	}
}

func TestProvisioningURL(t *testing.T) {
	u := ProvisioningURL("Sweetcorn", "a@b.c", "SECRET")
	if !strings.HasPrefix(u, "otpauth://totp/") || !strings.Contains(u, "secret=SECRET") {
		t.Fatalf("url=%s", u)
	}
}
