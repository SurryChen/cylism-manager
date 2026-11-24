package network

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

type adoptionKubernetesFake struct{}

func (adoptionKubernetesFake) KubernetesAvailable() bool { return true }
func (adoptionKubernetesFake) GetCertificateContext(context.Context, string, string) (*k8sclient.CertInfo, error) {
	return nil, fmt.Errorf("certificate pending")
}
func (adoptionKubernetesFake) DeleteTLSSecret(context.Context, string, string) error { return nil }
func (adoptionKubernetesFake) NamespaceExists(context.Context, string) (bool, error) {
	return true, nil
}
func (adoptionKubernetesFake) ListIssuersContext(context.Context) ([]k8sclient.IssuerInfo, error) {
	return []k8sclient.IssuerInfo{{Name: "dns-prod", Kind: "ClusterIssuer", Ready: true}}, nil
}
func (adoptionKubernetesFake) ReadyIngressEndpoint(context.Context, string, string) (bool, error) {
	return false, nil
}

type domainReferenceFake struct{ count int64 }

func (f domainReferenceFake) CountApplicationEndpointsByDomain(uint) (int64, error) {
	return f.count, nil
}

func TestManagedDomainInfoForUsesOnlyReferenceReaderWithoutKubernetes(t *testing.T) {
	info := ManagedDomainInfoFor(context.Background(), &model.ManagedDomain{ID: 7, Hostname: "api.example.com"}, nil, domainReferenceFake{count: 3})
	if info.Hostname != "api.example.com" || info.ApplicationCount != 3 || info.CertificateError == "" {
		t.Fatalf("unexpected managed domain view: %#v", info)
	}
}

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
	client := &k8sclient.Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList"}, certificate)}

	router := gin.New()
	service := networkservice.NewService(s, s).WithCertificateAdapter(client)
	handler := NewDomainHandlerWithDependencies(NewDomainKubernetesAdapter(client), service)
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

func TestDomainHandlerImportReusesMatchingLegacyDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	clusterID := uint(7)
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod", ClusterID: clusterID}); err != nil {
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
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"name": "legacy-api", "namespace": "commerce-prod"},
		"spec":     map[string]interface{}{"dnsNames": []interface{}{"api.example.com"}, "secretName": "legacy-api-tls", "issuerRef": map[string]interface{}{"name": "legacy-dns", "kind": "ClusterIssuer"}},
	}}
	client := &k8sclient.Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList"}, certificate)}
	cloud := &fakeDomainCloud{records: []cloudservice.DNSRecord{{ID: "existing", RR: "api", Type: "A", Value: "203.0.113.33"}}}
	router := gin.New()
	handler := NewDomainHandlerWithDependencies(NewDomainKubernetesAdapter(client), networkservice.NewService(s, s).WithCertificateAdapter(client)).WithCloudDNS(nil, fakeDomainServers{{ID: 2, Host: "203.0.113.12", ClusterID: &clusterID}})
	handler.cloud = cloud
	router.GET("/api/domains/importable-cloud-domains", handler.ListImportableCloudDomains)
	router.POST("/api/domains/import", handler.ImportCertificate)
	candidates := serve(router, newJSONRequest(http.MethodGet, "/api/domains/importable-cloud-domains?environment_id=1", nil))
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), "api.example.com") {
		t.Fatalf("legacy import candidate = %d: %s", candidates.Code, candidates.Body.String())
	}
	response := serve(router, newJSONRequest(http.MethodPost, "/api/domains/import", gin.H{"environment_id": 1, "certificate_name": "legacy-api", "enabled": true, "cloud_connection_id": 3, "dns_zone": "example.com", "node_ids": []uint{2}}))
	if response.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", response.Code, response.Body.String())
	}
	domain, err := s.GetManagedDomain(legacy.ID)
	if err != nil || domain.EnvironmentID != 1 || domain.CertificateOwnership != "imported" || domain.CertificateName != "legacy-api" || domain.TLSSecretName != "legacy-api-tls" || domain.AdoptionStatus != "staged" || cloud.creates != 0 || len(decodeDNSRecords(domain.OriginalDNSRecords)) != 1 {
		t.Fatalf("unexpected imported legacy domain: %#v, err=%v, cloud creates=%d", domain, err, cloud.creates)
	}
}

func TestAdoptCloudDomainDoesNotChangeExistingDNS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	clusterID := uint(7)
	if err := st.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod", ClusterID: clusterID}); err != nil {
		t.Fatal(err)
	}
	cloud := &fakeDomainCloud{records: []cloudservice.DNSRecord{{ID: "old", RR: "api", Type: "A", Value: "203.0.113.33"}}}
	handler := NewDomainHandlerWithDependencies(adoptionKubernetesFake{}, networkservice.NewService(st, st)).WithCloudDNS(nil, fakeDomainServers{{ID: 2, Host: "203.0.113.12", ClusterID: &clusterID}})
	handler.cloud = cloud
	router := gin.New()
	router.GET("/api/domains/adoptable-cloud-domains", handler.ListAdoptableCloudDomains)
	router.POST("/api/domains/adopt", handler.AdoptCloudDomain)
	router.PUT("/api/domains/:id", handler.Update)
	router.POST("/api/domains/:id/dns", handler.RetryDNS)
	candidates := serve(router, newJSONRequest(http.MethodGet, "/api/domains/adoptable-cloud-domains?environment_id=1", nil))
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), "api.example.com") {
		t.Fatalf("candidate response = %d: %s", candidates.Code, candidates.Body.String())
	}
	response := serve(router, newJSONRequest(http.MethodPost, "/api/domains/adopt", gin.H{"hostname": "api.example.com", "environment_id": 1, "issuer_ref": "dns-prod", "cloud_connection_id": 3, "dns_zone": "example.com", "node_ids": []uint{2}}))
	if response.Code != http.StatusOK {
		t.Fatalf("adopt response = %d: %s", response.Code, response.Body.String())
	}
	domain, err := st.GetManagedDomain(1)
	if err != nil || domain.AdoptionStatus != "staged" || domain.DNSRecords != "" || len(decodeDNSRecords(domain.OriginalDNSRecords)) != 1 {
		t.Fatalf("adopted domain = %#v, err=%v", domain, err)
	}
	updated := serve(router, newJSONRequest(http.MethodPut, "/api/domains/1", gin.H{"hostname": "api.example.com", "environment_id": 1, "issuer_ref": "dns-prod", "enabled": true, "node_ids": []uint{2}}))
	domain, err = st.GetManagedDomain(1)
	if updated.Code != http.StatusOK || err != nil || domain.AdoptionStatus != "staged" || domain.DNSStatus != "staged" || len(decodeDNSRecords(domain.OriginalDNSRecords)) != 1 || cloud.creates != 0 {
		t.Fatalf("editing adoption draft changed DNS: response=%d, domain=%#v, err=%v, creates=%d", updated.Code, domain, err, cloud.creates)
	}
	retry := serve(router, newJSONRequest(http.MethodPost, "/api/domains/1/dns", nil))
	if retry.Code == http.StatusOK || cloud.creates != 0 || len(cloud.records) != 1 || cloud.records[0].ID != "old" {
		t.Fatalf("staged DNS changed: response=%d, records=%#v", retry.Code, cloud.records)
	}
}
