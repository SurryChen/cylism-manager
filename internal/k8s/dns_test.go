package k8s

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestAliDNSProviderIsRegisteredAndRendersControlledSolver(t *testing.T) {
	provider, ok := GetDNSProvider("alidns")
	if !ok {
		t.Fatal("AliDNS provider is not registered")
	}
	if err := provider.ValidateValues(map[string]string{"access_key_id": "id", "access_key_secret": "secret"}); err != nil {
		t.Fatal(err)
	}
	solver := provider.Solver("cylism-dns-alidns-1")
	if provider.CredentialSecretName(solver) != "cylism-dns-alidns-1" {
		t.Fatalf("unexpected solver: %#v", solver)
	}
}

func TestUnknownDNSProviderIsRejected(t *testing.T) {
	if _, ok := GetDNSProvider("tencentcloud"); ok {
		t.Fatal("unknown provider must not be registered")
	}
}

func TestDNSCredentialSecretLifecycle(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	ctx := context.Background()
	values := map[string]string{"access_key_id": "id-1", "access_key_secret": "secret-1"}
	if err := client.UpsertDNSCredentialSecretContext(ctx, "alidns", "apps", "dns-credentials", values); err != nil {
		t.Fatal(err)
	}
	secret, err := client.Clientset.CoreV1().Secrets("apps").Get(ctx, "dns-credentials", metav1.GetOptions{})
	if err != nil || secret.StringData["access-key-id"] != "id-1" || secret.Labels["cylism.io/dns-provider"] != "alidns" {
		t.Fatalf("unexpected created secret: %#v %v", secret, err)
	}
	values["access_key_secret"] = "secret-2"
	if err := client.UpsertDNSCredentialSecretContext(ctx, "alidns", "apps", "dns-credentials", values); err != nil {
		t.Fatal(err)
	}
	secret, err = client.Clientset.CoreV1().Secrets("apps").Get(ctx, "dns-credentials", metav1.GetOptions{})
	if err != nil || secret.StringData["access-key-secret"] != "secret-2" {
		t.Fatalf("unexpected updated secret: %#v %v", secret, err)
	}
	if err := client.DeleteDNSCredentialSecretContext(ctx, "apps", "dns-credentials"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteDNSCredentialSecretContext(ctx, "apps", "dns-credentials"); err != nil {
		t.Fatal(err)
	}
}

func TestDNSCredentialSecretRejectsUnknownProviderAndInvalidValues(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	if err := client.UpsertDNSCredentialSecretContext(context.Background(), "unknown", "apps", "dns", nil); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("expected provider error, got %v", err)
	}
	if err := client.UpsertDNSCredentialSecretContext(context.Background(), "alidns", "apps", "dns", map[string]string{}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := (&Client{}).DeleteDNSCredentialSecretContext(context.Background(), "apps", "dns"); err == nil {
		t.Fatal("expected uninitialized client error")
	}
}
