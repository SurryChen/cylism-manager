package registry

import "testing"

func TestCredentialRoundTripNeverReturnsPlaintextAtRest(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	encrypted, err := EncryptCredential(key, "registry-password")
	if err != nil {
		t.Fatalf("EncryptCredential returned error: %v", err)
	}
	if encrypted == "" || encrypted == "registry-password" || !CredentialConfigured(encrypted) {
		t.Fatalf("credential was not safely represented: %q", encrypted)
	}
	plain, err := DecryptCredential(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptCredential returned error: %v", err)
	}
	if plain != "registry-password" {
		t.Fatalf("decrypted credential = %q", plain)
	}
}
