package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceEndpointInfo Service + Endpoint 汇总信息
type ServiceEndpointInfo struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Type          string            `json:"type"`
	ClusterIP     string            `json:"cluster_ip"`
	Ports         []string          `json:"ports"`
	EndpointCount int               `json:"endpoint_count"`
	Selector      map[string]string `json:"selector"`
	Age           string            `json:"age"`
}

// EndpointSliceInfo EndpointSlice 展示信息
type EndpointSliceInfo struct {
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	AddressType string          `json:"address_type"`
	Endpoints   []EndpointPoint `json:"endpoints"`
}

// EndpointPoint 单个端点
type EndpointPoint struct {
	IP      string `json:"ip"`
	Node    string `json:"node"`
	PodName string `json:"pod_name"`
	Ready   bool   `json:"ready"`
	Port    int32  `json:"port"`
}

// ListServices 列出所有 Service（扩展端点数量）
func (c *Client) ListServicesContext(ctx context.Context, ns string) ([]ServiceEndpointInfo, error) {
	var list *corev1.ServiceList
	var err error
	if ns == "" {
		list, err = c.Clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	result := make([]ServiceEndpointInfo, 0, len(list.Items))
	for _, s := range list.Items {
		info := serviceToEndpointInfo(&s)
		// Try to get endpoint count from EndpointSlices or Endpoints
		info.EndpointCount = c.countServiceEndpointsContext(ctx, s.Namespace, s.Name)
		result = append(result, info)
	}
	return result, nil
}

// GetService 获取单个 Service 详情
func (c *Client) GetServiceContext(ctx context.Context, namespace, name string) (*ServiceEndpointInfo, error) {
	s, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get service %s/%s: %w", namespace, name, err)
	}
	info := serviceToEndpointInfo(s)
	info.EndpointCount = c.countServiceEndpointsContext(ctx, namespace, name)
	return &info, nil
}

// GetServiceEndpoints 获取 Service 关联的 EndpointSlice（优先）或传统 Endpoints
func (c *Client) GetServiceEndpointsContext(ctx context.Context, namespace, name string) ([]EndpointSliceInfo, error) {
	// 优先尝试 EndpointSlice v1
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/service-name=%s", name),
	})
	if err == nil && len(slices.Items) > 0 {
		result := make([]EndpointSliceInfo, 0, len(slices.Items))
		for _, slice := range slices.Items {
			eps := make([]EndpointPoint, 0, len(slice.Endpoints))
			for _, ep := range slice.Endpoints {
				for _, addr := range ep.Addresses {
					podName := ""
					if ep.TargetRef != nil && ep.TargetRef.Kind == "Pod" {
						podName = ep.TargetRef.Name
					}
					port := int32(0)
					if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
						port = *slice.Ports[0].Port
					}
					eps = append(eps, EndpointPoint{
						IP:      addr,
						Node:    nodeNameFromPtr(ep.NodeName),
						PodName: podName,
						Ready:   ep.Conditions.Ready != nil && *ep.Conditions.Ready,
						Port:    port,
					})
				}
			}
			result = append(result, EndpointSliceInfo{
				Name:        slice.Name,
				Namespace:   slice.Namespace,
				AddressType: string(slice.AddressType),
				Endpoints:   eps,
			})
		}
		return result, nil
	}

	// Fallback to traditional Endpoints
	ep, err := c.Clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get endpoints: %w", err)
	}

	eps := make([]EndpointPoint, 0)
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			podName := ""
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				podName = addr.TargetRef.Name
			}
			port := int32(0)
			if len(subset.Ports) > 0 {
				port = subset.Ports[0].Port
			}
			eps = append(eps, EndpointPoint{
				IP:      addr.IP,
				Node:    nodeNameFromPtr(addr.NodeName),
				PodName: podName,
				Ready:   true,
				Port:    port,
			})
		}
	}

	return []EndpointSliceInfo{{
		Name:        ep.Name,
		Namespace:   ep.Namespace,
		AddressType: "IPv4",
		Endpoints:   eps,
	}}, nil
}

// countServiceEndpoints 数 Service 后端端点数量
func (c *Client) countServiceEndpointsContext(ctx context.Context, ns, name string) int {
	// Try EndpointSlice first
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/service-name=%s", name),
	})
	if err == nil {
		count := 0
		for _, s := range slices.Items {
			count += len(s.Endpoints)
		}
		return count
	}

	// Fallback
	ep, err := c.Clientset.CoreV1().Endpoints(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return 0
	}
	count := 0
	for _, subset := range ep.Subsets {
		count += len(subset.Addresses)
	}
	return count
}

func serviceToEndpointInfo(s *corev1.Service) ServiceEndpointInfo {
	ports := make([]string, 0, len(s.Spec.Ports))
	for _, p := range s.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%s:%d", p.Protocol, p.Port))
	}
	selector := make(map[string]string)
	for k, v := range s.Spec.Selector {
		selector[k] = v
	}
	return ServiceEndpointInfo{
		Name:      s.Name,
		Namespace: s.Namespace,
		Type:      string(s.Spec.Type),
		ClusterIP: s.Spec.ClusterIP,
		Ports:     ports,
		Selector:  selector,
		Age:       timeAgo(s.CreationTimestamp.Time),
	}
}

func nodeNameFromPtr(n *string) string {
	if n == nil {
		return "-"
	}
	return *n
}
