package api

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestDNSCredentialRequestEncryptsSecretAndKeepsItWhenBlank(t *testing.T) {
	st, err := store.New(t.TempDir() + "/cylism.db")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	h := NewCertHandlerWithEncryption(st, key)
	secret := "aliyun-secret"
	credential, plain, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", AccessKeyID: "LTAI", AccessKeySecret: &secret}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain != secret || credential.AccessKeySecret == secret {
		t.Fatalf("secret was not protected: %#v", credential)
	}
	credential.SecretName = "cylism-alidns-1"
	if err := st.CreateDNSCredential(credential); err != nil {
		t.Fatal(err)
	}
	updated, plain, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", AccessKeyID: "LTAI-2"}, credential)
	if err != nil {
		t.Fatal(err)
	}
	if plain != secret {
		t.Fatalf("blank update should retain the secret, got %q", plain)
	}
	decrypted, err := crypto.Decrypt(key, updated.AccessKeySecret)
	if err != nil || decrypted != secret {
		t.Fatalf("unexpected encrypted credential: %q, %v", decrypted, err)
	}
}
