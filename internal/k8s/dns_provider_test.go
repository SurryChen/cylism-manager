package k8s

import "testing"

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
