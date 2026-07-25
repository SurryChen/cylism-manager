package k8s

import (
	"fmt"
	"strconv"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressStdInfo 标准 Ingress 展示信息
type IngressStdInfo struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Hosts      []string `json:"hosts"`
	Paths      []string `json:"paths"`
	TLS        []string `json:"tls"`
	Controller string   `json:"controller"`
	Age        string   `json:"age"`
}

// IngressStdDetail 标准 Ingress 详情
type IngressStdDetail struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Rules       []IngressRuleInfo `json:"rules"`
	TLS         []string          `json:"tls"`
	Annotations map[string]string `json:"annotations"`
	Controller  string            `json:"controller"`
	Age         string            `json:"age"`
}

// IngressRuleInfo Ingress 规则
type IngressRuleInfo struct {
	Host  string            `json:"host"`
	Paths []IngressPathInfo `json:"paths"`
}

// IngressPathInfo Ingress 路径映射
type IngressPathInfo struct {
	Path        string `json:"path"`
	ServiceName string `json:"service_name"`
	ServicePort string `json:"service_port"`
}

// IngressControllerStatus Ingress Controller 状态
type IngressControllerStatus struct {
	Type      string `json:"type"`
	Version   string `json:"version"`
	Running   bool   `json:"running"`
	CRD       bool   `json:"crd"`
	Namespace string `json:"namespace"`
}

// ListIngresses 列出所有标准 Ingress
func (c *Client) ListIngresses(ns string) ([]IngressStdInfo, error) {
	var list *networkingv1.IngressList
	var err error
	if ns == "" {
		list, err = c.Clientset.NetworkingV1().Ingresses("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.NetworkingV1().Ingresses(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list ingresses: %w", err)
	}

	result := make([]IngressStdInfo, 0, len(list.Items))
	for _, ing := range list.Items {
		result = append(result, ingressStdToInfo(&ing))
	}
	return result, nil
}

// GetIngress 获取单个 Ingress 详情
func (c *Client) GetIngress(namespace, name string) (*IngressStdDetail, error) {
	ing, err := c.Clientset.NetworkingV1().Ingresses(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get ingress %s/%s: %w", namespace, name, err)
	}
	return ingressStdToDetail(ing), nil
}

// CreateIngress 创建 Ingress
func (c *Client) CreateIngress(namespace, name, host, path, svcName, svcPort string) (*IngressStdDetail, error) {
	pathType := networkingv1.PathTypePrefix
	backendPort := networkingv1.ServiceBackendPort{}
	if numPort, err := strconv.Atoi(svcPort); err == nil {
		backendPort.Number = int32(numPort)
	} else {
		backendPort.Name = svcPort
	}

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: svcName,
											Port: backendPort,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	created, err := c.Clientset.NetworkingV1().Ingresses(namespace).Create(c.ctx, ing, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create ingress: %w", err)
	}
	return ingressStdToDetail(created), nil
}

// DeleteIngress 删除 Ingress
func (c *Client) DeleteIngress(namespace, name string) error {
	return c.Clientset.NetworkingV1().Ingresses(namespace).Delete(c.ctx, name, metav1.DeleteOptions{})
}

// DetectIngressController 检测 Ingress Controller
func (c *Client) DetectIngressController() (*IngressControllerStatus, error) {
	// 1. Check Traefik CRD
	traefikCRD, _ := c.CheckCRD("ingressroutes.traefik.io")

	status := &IngressControllerStatus{
		Type: "Unknown",
		CRD:  traefikCRD,
	}

	deployList, err := c.Clientset.AppsV1().Deployments("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		// Can't query deployments — return what we have with the CRD error
		return status, fmt.Errorf("list deployments: %w", err)
	}

	// Try Traefik
	for _, d := range deployList.Items {
		if strings.Contains(d.Name, "traefik") {
			status.Type = "Traefik"
			status.Namespace = d.Namespace
			status.Running = d.Status.ReadyReplicas > 0
			for _, c := range d.Spec.Template.Spec.Containers {
				if strings.Contains(c.Image, "traefik") {
					parts := strings.SplitN(c.Image, ":", 2)
					if len(parts) == 2 {
						status.Version = parts[1]
					}
				}
			}
		}
	}

	// Fallback: NGINX Ingress
	if status.Type == "Unknown" {
		for _, d := range deployList.Items {
			if strings.Contains(d.Name, "nginx-ingress") || strings.Contains(d.Name, "ingress-nginx") {
				status.Type = "NGINX Ingress"
				status.Namespace = d.Namespace
				status.Running = d.Status.ReadyReplicas > 0
				break
			}
		}
	}

	return status, nil
}

func ingressStdToInfo(ing *networkingv1.Ingress) IngressStdInfo {
	hosts := make([]string, 0)
	paths := make([]string, 0)
	tls := make([]string, 0)

	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
		if rule.HTTP != nil {
			for _, p := range rule.HTTP.Paths {
				svcPort := ""
				if p.Backend.Service != nil {
					svcPort = p.Backend.Service.Port.Name
					if svcPort == "" {
						svcPort = fmt.Sprintf("%d", p.Backend.Service.Port.Number)
					}
					paths = append(paths, fmt.Sprintf("%s -> %s:%s", p.Path, p.Backend.Service.Name, svcPort))
				}
			}
		}
	}

	for _, t := range ing.Spec.TLS {
		for _, h := range t.Hosts {
			tls = append(tls, fmt.Sprintf("%s(%s)", h, t.SecretName))
		}
	}

	controller := ""
	if v, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
		controller = v
	}

	return IngressStdInfo{
		Name:       ing.Name,
		Namespace:  ing.Namespace,
		Hosts:      hosts,
		Paths:      paths,
		TLS:        tls,
		Controller: controller,
		Age:        timeAgo(ing.CreationTimestamp.Time),
	}
}

func ingressStdToDetail(ing *networkingv1.Ingress) *IngressStdDetail {
	rules := make([]IngressRuleInfo, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		ruleInfo := IngressRuleInfo{Host: rule.Host}
		if rule.HTTP != nil {
			for _, p := range rule.HTTP.Paths {
				svcName := ""
				svcPort := ""
				if p.Backend.Service != nil {
					svcName = p.Backend.Service.Name
					svcPort = p.Backend.Service.Port.Name
					if svcPort == "" {
						svcPort = fmt.Sprintf("%d", p.Backend.Service.Port.Number)
					}
				}
				ruleInfo.Paths = append(ruleInfo.Paths, IngressPathInfo{
					Path:        p.Path,
					ServiceName: svcName,
					ServicePort: svcPort,
				})
			}
		}
		rules = append(rules, ruleInfo)
	}

	tls := make([]string, 0)
	for _, t := range ing.Spec.TLS {
		tls = append(tls, t.SecretName)
	}

	controller := ""
	if v, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
		controller = v
	}

	annotations := make(map[string]string)
	for k, v := range ing.Annotations {
		annotations[k] = v
	}

	return &IngressStdDetail{
		Name:        ing.Name,
		Namespace:   ing.Namespace,
		Rules:       rules,
		TLS:         tls,
		Annotations: annotations,
		Controller:  controller,
		Age:         timeAgo(ing.CreationTimestamp.Time),
	}
}
