package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

var ingressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

// IngressRouteInfo 前端展示用
type IngressRouteInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Domain    string `json:"domain"`
	TLS       string `json:"tls"`
	CreatedAt string `json:"created_at"`
}

// ListIngressRoutes 列出所有 IngressRoute（所有 namespace）
func (c *Client) ListIngressRoutes() ([]IngressRouteInfo, error) {
	dynamicClient, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return nil, err
	}

	list, err := dynamicClient.Resource(ingressRouteGVR).Namespace("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list ingressroutes: %w", err)
	}

	var result []IngressRouteInfo
	for _, item := range list.Items {
		info := ingressRouteToInfo(&item)
		result = append(result, info)
	}
	return result, nil
}

// DeleteIngressRoute 删除 IngressRoute
func (c *Client) DeleteIngressRoute(namespace, name string) error {
	dynamicClient, _ := dynamic.NewForConfig(c.Config)
	return dynamicClient.Resource(ingressRouteGVR).Namespace(namespace).Delete(c.ctx, name, metav1.DeleteOptions{})
}

func ingressRouteToInfo(item *unstructured.Unstructured) IngressRouteInfo {
	info := IngressRouteInfo{
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04"),
	}

	// 提取域名
	if routes, ok := item.Object["spec"].(map[string]interface{})["routes"].([]interface{}); ok && len(routes) > 0 {
		if route, ok := routes[0].(map[string]interface{}); ok {
			if match, ok := route["match"].(string); ok {
				// 从 Host(\`example.com\`) 提取
				info.Domain = match
			}
		}
	}

	// TLS
	if tls, ok := item.Object["spec"].(map[string]interface{})["tls"]; ok {
		info.TLS = fmt.Sprintf("%v", tls)
	}

	return info
}
