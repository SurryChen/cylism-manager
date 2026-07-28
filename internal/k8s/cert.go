package k8s

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var certGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

var issuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
var clusterIssuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "clusterissuers"}

// CertInfo 证书展示信息
type CertInfo struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Domains    []string `json:"domains"`
	Status     string   `json:"status"` // Ready / Issuing / Failed
	ExpiryDate string   `json:"expiry_date"`
	CreatedAt  string   `json:"created_at"`
	Issuer     string   `json:"issuer"`
	IssuerKind string   `json:"issuer_kind"`
	SecretName string   `json:"secret_name"`
	Reason     string   `json:"reason,omitempty"`
}

type IssuerInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind"`
	Ready     bool   `json:"ready"`
	Reason    string `json:"reason,omitempty"`
}
type CreateCertificateRequest struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Domains    []string `json:"domains"`
	IssuerRef  string   `json:"issuer_ref"`
	IssuerKind string   `json:"issuer_kind"`
}

// ListCertificates 列出所有 Certificate
func (c *Client) ListCertificates() ([]CertInfo, error) {
	dynamicClient, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return nil, err
	}

	list, err := dynamicClient.Resource(certGVR).Namespace("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}

	var result []CertInfo
	for _, item := range list.Items {
		info := certToInfo(&item)
		result = append(result, info)
	}
	return result, nil
}

func (c *Client) ListIssuers() ([]IssuerInfo, error) {
	dynamicClient, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return nil, err
	}
	result := []IssuerInfo{}
	for _, source := range []struct {
		gvr        schema.GroupVersionResource
		kind       string
		namespaced bool
	}{{issuerGVR, "Issuer", true}, {clusterIssuerGVR, "ClusterIssuer", false}} {
		resource := dynamicClient.Resource(source.gvr)
		var list *unstructured.UnstructuredList
		if source.namespaced {
			list, err = resource.Namespace("").List(c.ctx, metav1.ListOptions{})
		} else {
			list, err = resource.List(c.ctx, metav1.ListOptions{})
		}
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", source.kind, err)
		}
		for i := range list.Items {
			result = append(result, issuerToInfo(&list.Items[i], source.kind))
		}
	}
	return result, nil
}

func issuerToInfo(item *unstructured.Unstructured, kind string) IssuerInfo {
	info := IssuerInfo{Name: item.GetName(), Namespace: item.GetNamespace(), Kind: kind}
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, raw := range conditions {
			if condition, ok := raw.(map[string]interface{}); ok && condition["type"] == "Ready" {
				info.Ready = condition["status"] == "True"
				info.Reason, _, _ = unstructured.NestedString(condition, "reason")
			}
		}
	}
	return info
}

func (c *Client) CreateCertificate(request CreateCertificateRequest) (*CertInfo, error) {
	dynamicClient, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return nil, err
	}
	issuerKind := request.IssuerKind
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": map[string]interface{}{"name": request.Name, "namespace": request.Namespace}, "spec": map[string]interface{}{"secretName": request.Name + "-tls", "dnsNames": stringSlice(request.Domains), "issuerRef": map[string]interface{}{"name": request.IssuerRef, "kind": issuerKind}}}}
	created, err := dynamicClient.Resource(certGVR).Namespace(request.Namespace).Create(c.ctx, object, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	info := certToInfo(created)
	return &info, nil
}

func stringSlice(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

// DeleteCertificate 删除 Certificate
func (c *Client) DeleteCertificate(namespace, name string) error {
	dynamicClient, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return err
	}
	return dynamicClient.Resource(certGVR).Namespace(namespace).Delete(c.ctx, name, metav1.DeleteOptions{})
}

func certToInfo(item *unstructured.Unstructured) CertInfo {
	info := CertInfo{
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04"),
	}

	// DNS names
	if spec, ok := item.Object["spec"].(map[string]interface{}); ok {
		if dnsNames, ok := spec["dnsNames"].([]interface{}); ok {
			for _, d := range dnsNames {
				info.Domains = append(info.Domains, fmt.Sprintf("%v", d))
			}
		}
		info.Issuer, _, _ = unstructured.NestedString(spec, "issuerRef", "name")
		info.IssuerKind, _, _ = unstructured.NestedString(spec, "issuerRef", "kind")
		info.SecretName, _, _ = unstructured.NestedString(spec, "secretName")
	}

	// Status
	if status, ok := item.Object["status"].(map[string]interface{}); ok {
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, c := range conditions {
				if cond, ok := c.(map[string]interface{}); ok {
					if cond["type"] == "Ready" && cond["status"] == "True" {
						info.Status = "Ready"
					}
				}
			}
		}
		if info.Status == "" {
			info.Status = "Issuing"
		}
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, raw := range conditions {
				if cond, ok := raw.(map[string]interface{}); ok && cond["type"] == "Ready" {
					if reason, ok := cond["reason"].(string); ok {
						info.Reason = reason
					}
					if cond["status"] == "False" {
						info.Status = "Failed"
					}
				}
			}
		}
		if notAfter, ok := status["notAfter"].(string); ok {
			info.ExpiryDate = notAfter
		}
	}

	return info
}
