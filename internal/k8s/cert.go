package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var certGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

// CertInfo 证书展示信息
type CertInfo struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Domains    []string `json:"domains"`
	Status     string   `json:"status"` // Ready / Issuing / Failed
	ExpiryDate string   `json:"expiry_date"`
	CreatedAt  string   `json:"created_at"`
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

// DeleteCertificate 删除 Certificate
func (c *Client) DeleteCertificate(namespace, name string) error {
	dynamicClient, _ := dynamic.NewForConfig(c.Config)
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
		if notAfter, ok := status["notAfter"].(string); ok {
			info.ExpiryDate = notAfter
		}
	}

	return info
}
