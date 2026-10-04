package srs

import "testing"

func TestEncodeDecode(t *testing.T) {
	secret := []byte("test-secret")
	enc := Encode(secret, "alice@example.com", "mail.relay.com")
	if enc == "alice@example.com" {
		t.Fatal("应被重写")
	}
	got, ok := Decode(secret, enc)
	if !ok || got != "alice@example.com" {
		t.Fatalf("Decode=%q ok=%v", got, ok)
	}
	// 篡改哈希 / 域名 应失败
	if _, ok := Decode(secret, "SRS0=XXXX=0102=example.com=alice@mail.relay.com"); ok {
		t.Fatal("错误哈希应失败")
	}
	if _, ok := Decode([]byte("other"), enc); ok {
		t.Fatal("密钥不符应失败")
	}
	if _, ok := Decode(secret, "bob@example.com"); ok {
		t.Fatal("非 SRS 应失败")
	}
}
