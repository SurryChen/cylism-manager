package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCertToInfoIncludesIssuerSecretAndFailure(t *testing.T) {
	certificate := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "api-example", "namespace": "production"},
		"spec": map[string]interface{}{
			"dnsNames":   []interface{}{"api.example.com", "www.example.com"},
			"secretName": "api-example-tls",
			"issuerRef":  map[string]interface{}{"name": "letsencrypt-dns", "kind": "ClusterIssuer"},
		},
		"status": map[string]interface{}{
			"notAfter":   "2026-10-01T12:00:00Z",
			"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "False", "reason": "Pending"}},
		},
	}}

	info := certToInfo(certificate)
	if info.Name != "api-example" || info.Namespace != "production" {
		t.Fatalf("unexpected identity: %#v", info)
	}
	if info.Issuer != "letsencrypt-dns" || info.IssuerKind != "ClusterIssuer" || info.SecretName != "api-example-tls" {
		t.Fatalf("issuer or secret was not extracted: %#v", info)
	}
	if info.Status != "Failed" || info.Reason != "Pending" || info.ExpiryDate != "2026-10-01T12:00:00Z" {
		t.Fatalf("unexpected status: %#v", info)
	}
	if len(info.Domains) != 2 || info.Domains[0] != "api.example.com" {
		t.Fatalf("unexpected domains: %#v", info.Domains)
	}
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
