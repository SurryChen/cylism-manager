package api

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestDomainFromRequestBuildsNamespaceBoundClusterIssuer(t *testing.T) {
	environment := &model.Environment{ID: 9, ProjectID: 3, Namespace: "production"}
	domain, err := domainFromRequest(domainRequest{Hostname: "API.Example.com", EnvironmentID: environment.ID, IssuerRef: "letsencrypt-prod", Enabled: true}, nil, environment)
	if err != nil {
		t.Fatal(err)
	}
	if domain.Hostname != "api.example.com" || domain.EnvironmentID != environment.ID || domain.Namespace != "production" || domain.IssuerKind != "ClusterIssuer" {
		t.Fatalf("unexpected managed domain: %#v", domain)
	}
}

func TestDomainFromRequestRejectsWildcardAndNamespaceMove(t *testing.T) {
	production := &model.Environment{ID: 9, Namespace: "production"}
	staging := &model.Environment{ID: 10, Namespace: "staging"}
	if _, err := domainFromRequest(domainRequest{Hostname: "*.example.com", EnvironmentID: production.ID, IssuerRef: "letsencrypt-prod"}, nil, production); err == nil {
		t.Fatal("expected wildcard to be rejected")
	}
	current := &model.ManagedDomain{ID: 4, Hostname: "api.example.com", EnvironmentID: production.ID, Namespace: "production", CertificateName: "cylism-domain-4", TLSSecretName: "cylism-domain-4-tls"}
	if _, err := domainFromRequest(domainRequest{Hostname: "api.example.com", EnvironmentID: staging.ID, IssuerRef: "letsencrypt-prod"}, current, staging); err == nil {
		t.Fatal("expected environment move to be rejected")
	}
}
