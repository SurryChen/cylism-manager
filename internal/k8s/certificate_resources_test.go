package k8s

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestCertificateOperationsHonorCallerContext(t *testing.T) {
	client := &Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{certGVR: "CertificateList"})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListCertificatesContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestCertToInfoKeepsTransientReadyFalseConditionsIssuing(t *testing.T) {
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "api-example", "namespace": "production"},
		"spec": map[string]interface{}{
			"dnsNames":   []interface{}{"api.example.com", "www.example.com"},
			"secretName": "api-example-tls",
			"issuerRef":  map[string]interface{}{"name": "letsencrypt-dns", "kind": "ClusterIssuer"},
		},
		"status": map[string]interface{}{
			"notAfter":    "2026-10-01T12:00:00Z",
			"renewalTime": "2026-09-01T12:00:00Z",
			"conditions":  []interface{}{map[string]interface{}{"type": "Ready", "status": "False", "reason": "Pending"}},
		},
	}}

	info := certToInfo(certificate)
	if info.Name != "api-example" || info.Namespace != "production" {
		t.Fatalf("unexpected identity: %#v", info)
	}
	if info.Issuer != "letsencrypt-dns" || info.IssuerKind != "ClusterIssuer" || info.SecretName != "api-example-tls" {
		t.Fatalf("issuer or secret was not extracted: %#v", info)
	}
	if info.Status != "Issuing" || info.Reason != "Pending" || info.ExpiryDate != "2026-10-01T12:00:00Z" || info.RenewalTime != "2026-09-01T12:00:00Z" {
		t.Fatalf("unexpected status: %#v", info)
	}
	if len(info.Domains) != 2 || info.Domains[0] != "api.example.com" {
		t.Fatalf("unexpected domains: %#v", info.Domains)
	}
}

func TestCertToInfoMarksTerminalReadyFalseConditionFailed(t *testing.T) {
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "invalid-cert", "namespace": "production"},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "False", "reason": "Failed"}}},
	}}
	info := certToInfo(certificate)
	if info.Status != "Failed" || info.Reason != "Failed" {
		t.Fatalf("expected terminal certificate failure, got %#v", info)
	}
}

func TestCertToInfoMarksReadyTrueConditionReady(t *testing.T) {
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "ready-cert", "namespace": "production"},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "reason": "Issued"}}},
	}}
	info := certToInfo(certificate)
	if info.Status != "Ready" || info.Reason != "Issued" {
		t.Fatalf("expected ready certificate, got %#v", info)
	}
}

func TestIssuerObjectRendersHTTP01AndAliDNS(t *testing.T) {
	httpIssuer, err := issuerObject(IssuerRequest{Name: "letsencrypt", Kind: "ClusterIssuer", Mode: "acme_http01", Email: "ops@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	// Assert the actual solver map because unstructured paths cannot index a list.
	solvers, ok, _ := unstructured.NestedSlice(httpIssuer.Object, "spec", "acme", "solvers")
	if !ok || solvers[0].(map[string]interface{})["http01"] == nil {
		t.Fatalf("missing HTTP-01 solver: %#v", httpIssuer.Object)
	}

	dnsIssuer, err := issuerObject(IssuerRequest{Name: "alidns", Kind: "Issuer", Namespace: "prod", Mode: "acme_dns01", DNSProvider: "alidns", Email: "ops@example.com", CredentialSecretName: "cylism-dns-alidns-1"})
	if err != nil {
		t.Fatal(err)
	}
	solvers, ok, _ = unstructured.NestedSlice(dnsIssuer.Object, "spec", "acme", "solvers")
	if !ok || solvers[0].(map[string]interface{})["dns01"] == nil {
		t.Fatalf("missing DNS-01 solver: %#v", dnsIssuer.Object)
	}
}

func TestListCertificateOperationsFollowsOwnerReferences(t *testing.T) {
	objects := []runtime.Object{
		operationObject("certificaterequests", "request-1", "production", "Certificate", "api-cert", nil),
		operationObject("orders", "order-1", "production", "CertificateRequest", "request-1", nil),
		operationObject("challenges", "challenge-1", "production", "Order", "order-1", map[string]interface{}{"dnsName": "api.example.com", "type": "DNS-01"}),
	}
	listKinds := map[schema.GroupVersionResource]string{certificateRequestGVR: "CertificateRequestList", orderGVR: "OrderList", challengeGVR: "ChallengeList"}
	client := &Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)}
	operations, err := client.ListCertificateOperationsContext(context.Background(), "production", "api-cert")
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 3 || operations[2].Kind != "Challenge" || operations[2].Domain != "api.example.com" {
		t.Fatalf("unexpected operations: %#v", operations)
	}
}

func TestEnsureCertificateUsesManagedSecretAndIsIdempotent(t *testing.T) {
	listKinds := map[schema.GroupVersionResource]string{certGVR: "CertificateList"}
	client := &Client{DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)}
	request := CreateCertificateRequest{Name: "cylism-domain-7", Namespace: "production", Domains: []string{"api.example.com"}, IssuerRef: "letsencrypt-prod", IssuerKind: "ClusterIssuer", SecretName: "cylism-domain-7-tls"}
	if _, err := client.EnsureCertificateContext(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.IssuerRef = "letsencrypt-new"
	if _, err := client.EnsureCertificateContext(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	certificate, err := client.GetCertificateContext(context.Background(), "production", "cylism-domain-7")
	if err != nil {
		t.Fatal(err)
	}
	if certificate.SecretName != "cylism-domain-7-tls" || certificate.Issuer != "letsencrypt-new" {
		t.Fatalf("unexpected managed certificate: %#v", certificate)
	}
}

func operationObject(resource, name, namespace, ownerKind, ownerName string, spec map[string]interface{}) *unstructured.Unstructured {
	group := "cert-manager.io/v1"
	kind := "CertificateRequest"
	if resource == "orders" {
		group, kind = "acme.cert-manager.io/v1", "Order"
	}
	if resource == "challenges" {
		group, kind = "acme.cert-manager.io/v1", "Challenge"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": group, "kind": kind, "metadata": map[string]interface{}{"name": name, "namespace": namespace, "ownerReferences": []interface{}{map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": ownerKind, "name": ownerName, "uid": "owner"}}}, "spec": spec, "status": map[string]interface{}{"state": "pending"}}}
}

func TestIssuerToInfoReadsReadyCondition(t *testing.T) {
	issuer := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "letsencrypt-dns", "namespace": "production"},
		"status": map[string]interface{}{
			"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "reason": "Verified"}},
		},
	}}

	info := issuerToInfo(issuer, "Issuer")
	if !info.Ready || info.Kind != "Issuer" || info.Reason != "Verified" || info.Namespace != "production" {
		t.Fatalf("unexpected issuer info: %#v", info)
	}
}
