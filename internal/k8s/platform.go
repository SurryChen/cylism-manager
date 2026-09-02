package k8s

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	platformDeploymentNamespace = "default"
	platformDeploymentName      = "cylism-manager"
	platformContainerName       = "platform"
	platformReleaseAnnotation   = "cylism.io/platform-release"
	platformRestartAnnotation   = "cylism.io/platform-restarted-at"
	platformEndpointLabel       = "cylism.io/platform-endpoint"
	platformEndpointLabelValue  = "true"
)

// PlatformDeploymentStatus is the only supported self-update target state.
type PlatformDeploymentStatus struct {
	Image           string `json:"image"`
	ReleaseID       uint   `json:"release_id,omitempty"`
	DesiredReplicas int32  `json:"desired_replicas"`
	ReadyReplicas   int32  `json:"ready_replicas"`
	Failure         string `json:"failure,omitempty"`
}

// PlatformIngressInfo is the constrained route information managed for the
// platform's own public endpoint.
type PlatformIngressInfo struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	IngressClass  string `json:"ingress_class,omitempty"`
	Hostname      string `json:"hostname"`
	Path          string `json:"path"`
	ServiceName   string `json:"service_name"`
	ServicePort   string `json:"service_port"`
	TLSSecretName string `json:"tls_secret_name,omitempty"`
	Managed       bool   `json:"managed"`
}

func (c *Client) PlatformDeploymentStatusContext(ctx context.Context) (*PlatformDeploymentStatus, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(ctx, platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read platform deployment: %w", err)
	}
	status := &PlatformDeploymentStatus{DesiredReplicas: 1, ReadyReplicas: deployment.Status.AvailableReplicas}
	if deployment.Spec.Replicas != nil {
		status.DesiredReplicas = *deployment.Spec.Replicas
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == platformContainerName {
			status.Image = container.Image
			break
		}
	}
	if status.Image == "" {
		return nil, fmt.Errorf("platform deployment does not contain %q container", platformContainerName)
	}
	if raw := deployment.Spec.Template.Annotations[platformReleaseAnnotation]; raw != "" {
		value, _ := strconv.ParseUint(raw, 10, 64)
		status.ReleaseID = uint(value)
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == "True" {
			status.Failure = condition.Message
		}
	}
	return status, nil
}

// UpdatePlatformDeployment changes exactly one known Deployment/container pair.
func (c *Client) UpdatePlatformDeploymentContext(ctx context.Context, image string, releaseID uint) (string, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(ctx, platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read platform deployment: %w", err)
	}
	previous := ""
	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		if container.Name == platformContainerName {
			previous = container.Image
			container.Image = image
			break
		}
	}
	if previous == "" {
		return "", fmt.Errorf("platform deployment does not contain %q container", platformContainerName)
	}
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations[platformReleaseAnnotation] = strconv.FormatUint(uint64(releaseID), 10)
	deployment.Spec.Template.Annotations[platformRestartAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("update platform deployment: %w", err)
	}
	return previous, nil
}

// EnsurePlatformEndpoint reconciles the Ingress used to reach Cylism Manager
// itself. Certificate lifecycle remains in the central certificate manager.
func (c *Client) EnsurePlatformEndpointContext(ctx context.Context, hostname, tlsSecretName, ingressName string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := c.ensurePlatformIngress(ctx, hostname, tlsSecretName, ingressName); err != nil {
		return fmt.Errorf("同步平台 Ingress: %w", err)
	}
	return nil
}

func (c *Client) ensurePlatformIngress(ctx context.Context, hostname, tlsSecretName, ingressName string) error {
	ingressName = platformIngressName(ingressName)
	if err := c.ensurePlatformHostnameAvailable(ctx, hostname, ingressName); err != nil {
		return err
	}
	resource := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace)
	existing, err := resource.Get(ctx, ingressName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, platformIngress(ingressName, hostname, tlsSecretName), metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Labels[platformEndpointLabel] != platformEndpointLabelValue {
		return fmt.Errorf("目标 Ingress %s/%s 不受平台管理，请先显式接管", platformDeploymentNamespace, ingressName)
	}
	desired := platformIngress(ingressName, hostname, tlsSecretName)
	desired.ResourceVersion = existing.ResourceVersion
	desired.Labels = copyStringMap(existing.Labels)
	desired.Labels[platformEndpointLabel] = platformEndpointLabelValue
	desired.Labels["app.kubernetes.io/managed-by"] = "cylism-manager"
	desired.Annotations = copyStringMap(existing.Annotations)
	desired.Spec.IngressClassName = existing.Spec.IngressClassName
	_, err = resource.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

// AdoptPlatformIngress takes ownership only after verifying that the existing
// resource already routes the requested root host to the platform Service.
func (c *Client) AdoptPlatformIngressContext(ctx context.Context, hostname, tlsSecretName string) (string, error) {
	if c == nil || c.Clientset == nil {
		return "", fmt.Errorf("Kubernetes 客户端未初始化")
	}
	resource := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace)
	items, err := resource.List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("读取候选 Ingress: %w", err)
	}
	candidates := make([]*networkingv1.Ingress, 0, 1)
	for index := range items.Items {
		ingress := &items.Items[index]
		if matchesPlatformIngress(ingress, hostname) {
			candidates = append(candidates, ingress)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("未找到可接管的 Ingress；需位于 %s 命名空间，使用域名 %q、路径 / 并转发到 %s:8080", platformDeploymentNamespace, hostname, platformDeploymentName)
	}
	if len(candidates) > 1 {
		return "", fmt.Errorf("找到多个可接管的 Ingress，请先保留唯一的 %q 平台入口", hostname)
	}
	existing := candidates[0]
	if existing.Labels[platformEndpointLabel] != platformEndpointLabelValue {
		updated := existing.DeepCopy()
		updated.Labels = copyStringMap(updated.Labels)
		updated.Labels[platformEndpointLabel] = platformEndpointLabelValue
		updated.Labels["app.kubernetes.io/managed-by"] = "cylism-manager"
		if _, err := resource.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return "", err
		}
	}
	if err := c.ensurePlatformIngress(ctx, hostname, tlsSecretName, existing.Name); err != nil {
		return "", err
	}
	return existing.Name, nil
}

func (c *Client) ensurePlatformHostnameAvailable(ctx context.Context, hostname, managedIngressName string) error {
	ingresses, err := c.Clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("检查现有 Ingress: %w", err)
	}
	for _, ingress := range ingresses.Items {
		if ingress.Namespace == platformDeploymentNamespace && ingress.Name == managedIngressName {
			continue
		}
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == hostname {
				return fmt.Errorf("域名 %q 已被 Ingress %s/%s 使用", hostname, ingress.Namespace, ingress.Name)
			}
		}
	}
	return nil
}

// RemovePlatformEndpoint stops public routing without modifying certificates
// or TLS Secrets managed by the certificate subsystem.
func (c *Client) RemovePlatformEndpointContext(ctx context.Context, ingressName string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	resource := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace)
	ingressName = platformIngressName(ingressName)
	ingress, err := resource.Get(ctx, ingressName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ingress.Labels[platformEndpointLabel] != platformEndpointLabelValue {
		return fmt.Errorf("目标 Ingress %s/%s 不受平台管理", platformDeploymentNamespace, platformDeploymentName)
	}
	return resource.Delete(ctx, ingressName, metav1.DeleteOptions{})
}

func (c *Client) PlatformIngressReadyContext(ctx context.Context, ingressName string) (bool, error) {
	if c == nil || c.Clientset == nil {
		return false, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingressName = platformIngressName(ingressName)
	ingress, err := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace).Get(ctx, ingressName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ingress.Labels[platformEndpointLabel] == platformEndpointLabelValue, nil
}

// PlatformIngressInfo returns the route currently used for the platform
// endpoint. A missing Ingress is represented as nil so callers can distinguish
// it from an unavailable Kubernetes API.
func (c *Client) PlatformIngressInfoContext(ctx context.Context, ingressName string) (*PlatformIngressInfo, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingressName = platformIngressName(ingressName)
	ingress, err := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace).Get(ctx, ingressName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info := &PlatformIngressInfo{
		Name:      ingress.Name,
		Namespace: ingress.Namespace,
		Managed:   ingress.Labels[platformEndpointLabel] == platformEndpointLabelValue,
	}
	if ingress.Spec.IngressClassName != nil {
		info.IngressClass = *ingress.Spec.IngressClassName
	}
	if len(ingress.Spec.TLS) > 0 {
		info.TLSSecretName = ingress.Spec.TLS[0].SecretName
	}
	if len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].HTTP == nil || len(ingress.Spec.Rules[0].HTTP.Paths) == 0 {
		return info, nil
	}
	rule := ingress.Spec.Rules[0]
	path := rule.HTTP.Paths[0]
	info.Hostname = rule.Host
	info.Path = path.Path
	if path.Backend.Service != nil {
		info.ServiceName = path.Backend.Service.Name
		if path.Backend.Service.Port.Name != "" {
			info.ServicePort = path.Backend.Service.Port.Name
		} else if path.Backend.Service.Port.Number != 0 {
			info.ServicePort = strconv.Itoa(int(path.Backend.Service.Port.Number))
		}
	}
	return info, nil
}

func matchesPlatformIngress(ingress *networkingv1.Ingress, hostname string) bool {
	if ingress == nil || len(ingress.Spec.Rules) != 1 || ingress.Spec.Rules[0].Host != hostname {
		return false
	}
	http := ingress.Spec.Rules[0].IngressRuleValue.HTTP
	if http == nil || len(http.Paths) != 1 {
		return false
	}
	path := http.Paths[0]
	return path.Path == "/" && path.Backend.Service != nil && path.Backend.Service.Name == platformDeploymentName && path.Backend.Service.Port.Number == 8080
}

func copyStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func platformIngressName(name string) string {
	if name == "" {
		return platformDeploymentName
	}
	return name
}

func platformIngress(ingressName, hostname, tlsSecretName string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: platformIngressName(ingressName), Namespace: platformDeploymentNamespace, Labels: map[string]string{platformEndpointLabel: platformEndpointLabelValue, "app.kubernetes.io/managed-by": "cylism-manager"}},
		Spec: networkingv1.IngressSpec{
			TLS:   []networkingv1.IngressTLS{{Hosts: []string{hostname}, SecretName: tlsSecretName}},
			Rules: []networkingv1.IngressRule{{Host: hostname, IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: platformDeploymentName, Port: networkingv1.ServiceBackendPort{Number: 8080}}}}}}}}},
		},
	}
}
