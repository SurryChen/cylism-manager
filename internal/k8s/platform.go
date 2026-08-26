package k8s

import (
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

	// PlatformEndpointCertificateName and PlatformEndpointTLSSecretName are
	// fixed resources in the control-plane namespace, not application assets.
	PlatformEndpointCertificateName = "cylism-manager-tls"
	PlatformEndpointTLSSecretName   = "cylism-manager-tls"
)

// PlatformDeploymentStatus is the only supported self-update target state.
type PlatformDeploymentStatus struct {
	Image           string `json:"image"`
	ReleaseID       uint   `json:"release_id,omitempty"`
	DesiredReplicas int32  `json:"desired_replicas"`
	ReadyReplicas   int32  `json:"ready_replicas"`
	Failure         string `json:"failure,omitempty"`
}

func (c *Client) PlatformDeploymentStatus() (*PlatformDeploymentStatus, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
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
func (c *Client) UpdatePlatformDeployment(image string, releaseID uint) (string, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
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
	if _, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("update platform deployment: %w", err)
	}
	return previous, nil
}

// EnsurePlatformEndpoint reconciles the Certificate and Ingress used to reach
// Cylism Manager itself. DNS and certificate ownership stay independent from
// project environments and application releases.
func (c *Client) EnsurePlatformEndpoint(hostname, issuerRef string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if err := c.ensurePlatformCertificate(CreateCertificateRequest{
		Name:       PlatformEndpointCertificateName,
		Namespace:  platformDeploymentNamespace,
		Domains:    []string{hostname},
		IssuerRef:  issuerRef,
		IssuerKind: "ClusterIssuer",
		SecretName: PlatformEndpointTLSSecretName,
	}); err != nil {
		return fmt.Errorf("同步平台 Certificate: %w", err)
	}
	if err := c.ensurePlatformIngress(hostname); err != nil {
		return fmt.Errorf("同步平台 Ingress: %w", err)
	}
	return nil
}

func (c *Client) ensurePlatformCertificate(request CreateCertificateRequest) error {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return err
	}
	resource := dynamicClient.Resource(certGVR).Namespace(request.Namespace)
	desired := certificateObject(request)
	desired.SetLabels(map[string]string{platformEndpointLabel: platformEndpointLabelValue, "app.kubernetes.io/managed-by": "cylism-manager"})
	existing, err := resource.Get(c.Ctx(), request.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(c.Ctx(), desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.GetLabels()[platformEndpointLabel] != platformEndpointLabelValue {
		return fmt.Errorf("目标 Certificate %s/%s 不受平台管理", request.Namespace, request.Name)
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(c.Ctx(), desired, metav1.UpdateOptions{})
	return err
}

func (c *Client) ensurePlatformIngress(hostname string) error {
	ingresses, err := c.Clientset.NetworkingV1().Ingresses("").List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("检查现有 Ingress: %w", err)
	}
	for _, ingress := range ingresses.Items {
		if ingress.Namespace == platformDeploymentNamespace && ingress.Name == platformDeploymentName {
			continue
		}
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == hostname {
				return fmt.Errorf("域名 %q 已被 Ingress %s/%s 使用", hostname, ingress.Namespace, ingress.Name)
			}
		}
	}

	desired := platformIngress(hostname)
	resource := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace)
	existing, err := resource.Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(c.Ctx(), desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Labels[platformEndpointLabel] != platformEndpointLabelValue {
		return fmt.Errorf("目标 Ingress %s/%s 不受平台管理", platformDeploymentNamespace, platformDeploymentName)
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = resource.Update(c.Ctx(), desired, metav1.UpdateOptions{})
	return err
}

// RemovePlatformEndpoint stops public routing while retaining the Certificate
// and TLS Secret for a later re-enable.
func (c *Client) RemovePlatformEndpoint() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	resource := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace)
	ingress, err := resource.Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ingress.Labels[platformEndpointLabel] != platformEndpointLabelValue {
		return fmt.Errorf("目标 Ingress %s/%s 不受平台管理", platformDeploymentNamespace, platformDeploymentName)
	}
	return resource.Delete(c.Ctx(), platformDeploymentName, metav1.DeleteOptions{})
}

func (c *Client) PlatformIngressReady() (bool, error) {
	if c == nil || c.Clientset == nil {
		return false, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingress, err := c.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace).Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ingress.Labels[platformEndpointLabel] == platformEndpointLabelValue, nil
}

func platformIngress(hostname string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: platformDeploymentName, Namespace: platformDeploymentNamespace, Labels: map[string]string{platformEndpointLabel: platformEndpointLabelValue, "app.kubernetes.io/managed-by": "cylism-manager"}},
		Spec: networkingv1.IngressSpec{
			TLS:   []networkingv1.IngressTLS{{Hosts: []string{hostname}, SecretName: PlatformEndpointTLSSecretName}},
			Rules: []networkingv1.IngressRule{{Host: hostname, IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: platformDeploymentName, Port: networkingv1.ServiceBackendPort{Number: 8080}}}}}}}}},
		},
	}
}
