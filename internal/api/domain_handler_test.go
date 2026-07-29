package api

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestDomainFromRequestBuildsNamespaceBoundClusterIssuer(t *testing.T) {
	domain, err := domainFromRequest(domainRequest{Hostname: "API.Example.com", Namespace: "production", IssuerRef: "letsencrypt-prod", Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if domain.Hostname != "api.example.com" || domain.Namespace != "production" || domain.IssuerKind != "ClusterIssuer" {
		t.Fatalf("unexpected managed domain: %#v", domain)
	}
}

func TestDomainFromRequestRejectsWildcardAndNamespaceMove(t *testing.T) {
	if _, err := domainFromRequest(domainRequest{Hostname: "*.example.com", Namespace: "production", IssuerRef: "letsencrypt-prod"}, nil); err == nil {
		t.Fatal("expected wildcard to be rejected")
	}
	current := &model.ManagedDomain{ID: 4, Hostname: "api.example.com", Namespace: "production", CertificateName: "cylism-domain-4", TLSSecretName: "cylism-domain-4-tls"}
	if _, err := domainFromRequest(domainRequest{Hostname: "api.example.com", Namespace: "staging", IssuerRef: "letsencrypt-prod"}, current); err == nil {
		t.Fatal("expected namespace move to be rejected")
	}
}
