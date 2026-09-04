package security

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	plaintext := "my-secret-password"

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == plaintext {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("expected '%s', got '%s'", plaintext, decrypted)
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := []byte("0123456789abcdef0123456789abcdef")
	key2 := []byte("fedcba9876543210fedcba9876543210")

	ciphertext, _ := Encrypt(key1, "secret")
	_, err := Decrypt(key2, ciphertext)
	if err == nil {
		t.Error("expected error with wrong key")
	}
}

func TestEncryptDecryptEmpty(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")

	ciphertext, err := Encrypt(key, "")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if decrypted != "" {
		t.Errorf("expected empty, got '%s'", decrypted)
	}
}

func TestEncryptDecryptLongText(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----\nvery long key content here"

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt long: %v", err)
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt long: %v", err)
	}
	if decrypted != plaintext {
		t.Error("long text roundtrip failed")
	}
}
