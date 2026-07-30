package api

import (
	"net/http"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
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

func TestDomainHandlerImportsExistingCertificateWithoutTakingOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"name": "legacy-api", "namespace": "commerce-prod"},
		"spec":     map[string]interface{}{"dnsNames": []interface{}{"api.example.com"}, "secretName": "legacy-api-tls", "issuerRef": map[string]interface{}{"name": "legacy-dns", "kind": "ClusterIssuer"}},
	}}
	originalK8s := K8s
	K8s = &k8sclient.Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList"}, certificate)}
	defer func() { K8s = originalK8s }()

	router := gin.New()
	handler := NewDomainHandler(s)
	router.POST("/api/domains/import", handler.ImportCertificate)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/domains/import", gin.H{"environment_id": 1, "certificate_name": "legacy-api", "enabled": true}))
	if response.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", response.Code, response.Body.String())
	}
	domain, err := s.GetManagedDomain(1)
	if err != nil || domain.EnvironmentID != 1 || domain.CertificateOwnership != "imported" || domain.TLSSecretName != "legacy-api-tls" {
		t.Fatalf("unexpected imported domain: %#v, err=%v", domain, err)
	}
}

func TestDomainHandlerClaimBindsMatchingLegacyDomainToEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	legacy := &model.ManagedDomain{
		Hostname:        "api.example.com",
		Namespace:       "commerce-prod",
		CertificateName: "legacy-api",
		TLSSecretName:   "legacy-api-tls",
		IssuerRef:       "legacy-dns",
		IssuerKind:      "ClusterIssuer",
		Enabled:         true,
	}
	if err := s.CreateManagedDomain(legacy); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler := NewDomainHandler(s)
	router.POST("/api/domains/:id/claim", handler.Claim)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/domains/1/claim", gin.H{"environment_id": 1}))
	if response.Code != http.StatusOK {
		t.Fatalf("claim status = %d: %s", response.Code, response.Body.String())
	}
	domain, err := s.GetManagedDomain(legacy.ID)
	if err != nil || domain.EnvironmentID != 1 || domain.CertificateOwnership != "managed" {
		t.Fatalf("unexpected recovered domain: %#v, err=%v", domain, err)
	}
}
