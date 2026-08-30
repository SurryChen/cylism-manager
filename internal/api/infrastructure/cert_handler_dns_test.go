package infrastructure

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
	secret, identifier := "aliyun-secret", "LTAI"
	credential, values, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"access_key_id": &identifier, "access_key_secret": &secret}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values["access_key_secret"] != secret || credential.EncryptedValues == secret {
		t.Fatalf("secret was not protected: %#v", credential)
	}
	credential.SecretName = "cylism-dns-alidns-1"
	if err := st.CreateDNSCredential(credential); err != nil {
		t.Fatal(err)
	}
	newIdentifier := "LTAI-2"
	updated, values, err := h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"access_key_id": &newIdentifier}}, credential)
	if err != nil {
		t.Fatal(err)
	}
	if values["access_key_secret"] != secret {
		t.Fatalf("blank update should retain the secret, got %q", values["access_key_secret"])
	}
	decrypted, err := crypto.Decrypt(key, updated.EncryptedValues)
	if err != nil || decrypted == secret {
		t.Fatalf("unexpected encrypted credential: %q, %v", decrypted, err)
	}
}

func TestDNSCredentialRequestRejectsUnknownProviderField(t *testing.T) {
	st, err := store.New(t.TempDir() + "/cylism.db")
	if err != nil {
		t.Fatal(err)
	}
	h := NewCertHandlerWithEncryption(st, []byte("01234567890123456789012345678901"))
	value := "unexpected"
	_, _, err = h.dnsCredentialFromRequest(dnsCredentialRequest{Name: "aliyun", Namespace: "cert-manager", Provider: "alidns", Values: map[string]*string{"unknown": &value}}, nil)
	if err == nil {
		t.Fatal("expected unknown provider field to be rejected")
	}
}
