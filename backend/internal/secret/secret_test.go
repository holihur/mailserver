package secret

import (
	"encoding/base64"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	SetKey("test-secret-key")
	enc, err := Encrypt([]byte(`{"api_token":"super-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if enc == `{"api_token":"super-secret"}` {
		t.Fatal("ciphertext should not equal plaintext")
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != `{"api_token":"super-secret"}` {
		t.Fatalf("round trip mismatch: %s", dec)
	}
}

func TestDifferentKeysFail(t *testing.T) {
	SetKey("key-a")
	enc, err := Encrypt([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	SetKey("key-b")
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestDecryptErrors(t *testing.T) {
	SetKey("k")
	if _, err := Decrypt("!!!not base64!!!"); err == nil {
		t.Fatal("bad base64 should error")
	}
	short := base64.StdEncoding.EncodeToString([]byte("abc"))
	if _, err := Decrypt(short); err == nil {
		t.Fatal("short ciphertext should error")
	}
}

func TestNilKey(t *testing.T) {
	mu.Lock()
	key = nil
	mu.Unlock()
	if _, err := Encrypt([]byte("x")); err == nil {
		t.Fatal("encrypt without key should error")
	}
	if _, err := Decrypt("AAAA"); err == nil {
		t.Fatal("decrypt without key should error")
	}
	SetKey("restore")
}
