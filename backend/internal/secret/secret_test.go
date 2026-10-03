package secret

import "testing"

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
