package application

import (
	"context"
	"fmt"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var certificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

type KubernetesApplier struct {
	Client           *k8sclient.Client
	ReadinessTimeout time.Duration
}

func NewKubernetesApplier(client *k8sclient.Client) *KubernetesApplier {
	return &KubernetesApplier{Client: client, ReadinessTimeout: 2 * time.Minute}
}

func (a *KubernetesApplier) Preflight(ctx context.Context, application ApplicationContext, endpoint EndpointSpec) error {
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	namespace, err := a.Client.Clientset.CoreV1().Namespaces().Get(ctx, application.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("环境命名空间 %q 不存在，请在环境页面创建或同步命名空间", application.Namespace)
	}
	if err != nil {
		return fmt.Errorf("检查环境命名空间: %w", err)
	}
	if namespace.Status.Phase != corev1.NamespaceActive {
		return fmt.Errorf("环境命名空间 %q 未就绪", application.Namespace)
	}
	if endpoint.Exposure != ExposurePublic {
		return nil
	}
	status, err := a.Client.DetectIngressController()
	if err != nil || status == nil || !status.Running {
		return fmt.Errorf("Ingress Controller 未就绪")
	}
	if endpoint.TLSEnabled {
		ok, err := a.Client.CheckCRD("certificates.cert-manager.io")
		if err != nil || !ok {
			return fmt.Errorf("cert-manager Certificate CRD 不可用")
		}
	}
	return nil
}

func (a *KubernetesApplier) Apply(ctx context.Context, resources *RenderedResources) error {
	if resources.ImagePullSecret != nil {
		if err := a.applySecret(ctx, resources.ImagePullSecret); err != nil {
			return err
		}
	}
	if resources.ConfigMap != nil {
		if err := a.applyConfigMap(ctx, resources.ConfigMap); err != nil {
			return err
		}
	}
	if resources.Secret != nil {
		if err := a.applySecret(ctx, resources.Secret); err != nil {
			return err
		}
	}
	if err := a.applyDeployment(ctx, resources.Deployment); err != nil {
		return err
	}
	if err := a.applyService(ctx, resources.Service); err != nil {
		return err
	}
	if resources.Certificate != nil {
		if err := a.applyCertificate(ctx, resources.Certificate); err != nil {
			return err
		}
	}
	if resources.Ingress != nil {
		if err := a.applyIngress(ctx, resources.Ingress); err != nil {
			return err
		}
	}
	return nil
}

// SyncEndpoint updates only the application Ingress. Domain ownership is
// application-level state, so it must not wait for a version release.
func (a *KubernetesApplier) SyncEndpoint(ctx context.Context, application ApplicationContext, endpoint EndpointSpec, servicePort int32) error {
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if endpoint.Exposure != ExposurePublic {
		return a.RemoveEndpoint(ctx, application)
	}
	return a.applyIngress(ctx, endpointIngress(application, endpoint, servicePort))
}

func (a *KubernetesApplier) RemoveEndpoint(ctx context.Context, application ApplicationContext) error {
	if a.Client == nil || a.Client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	ingress, err := a.Client.Clientset.NetworkingV1().Ingresses(application.Namespace).Get(ctx, application.ApplicationName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(ingress.Labels); err != nil {
		return err
	}
	return a.Client.Clientset.NetworkingV1().Ingresses(application.Namespace).Delete(ctx, application.ApplicationName, metav1.DeleteOptions{})
}

func (a *KubernetesApplier) WaitReady(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error {
	timeout := a.ReadinessTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		deployment, err := a.Client.Clientset.AppsV1().Deployments(application.Namespace).Get(deadline, application.ApplicationName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取 Deployment 就绪状态: %w", err)
		}
		if deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.UpdatedReplicas >= spec.Replicas && deployment.Status.AvailableReplicas >= spec.Replicas && deployment.Status.UnavailableReplicas == 0 {
			if !spec.Endpoint.TLSEnabled {
				return nil
			}
			ready, err := a.certificateReady(deadline, application, spec.Endpoint)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("等待工作负载就绪超时")
		case <-ticker.C:
		}
	}
}

func (a *KubernetesApplier) applyConfigMap(ctx context.Context, desired *corev1.ConfigMap) error {
	existing, err := a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applySecret(ctx context.Context, desired *corev1.Secret) error {
	existing, err := a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.CoreV1().Secrets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	existing, err := a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.AppsV1().Deployments(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyService(ctx context.Context, desired *corev1.Service) error {
	existing, err := a.Client.Clientset.CoreV1().Services(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.CoreV1().Services(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	_, err = a.Client.Clientset.CoreV1().Services(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyIngress(ctx context.Context, desired *networkingv1.Ingress) error {
	existing, err := a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.Client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) applyCertificate(ctx context.Context, desired *unstructured.Unstructured) error {
	dynamicClient, err := dynamic.NewForConfig(a.Client.Config)
	if err != nil {
		return err
	}
	resource := dynamicClient.Resource(certificateGVR).Namespace(desired.GetNamespace())
	existing, err := resource.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureManaged(existing.GetLabels()); err != nil {
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *KubernetesApplier) certificateReady(ctx context.Context, application ApplicationContext, endpoint EndpointSpec) (bool, error) {
	dynamicClient, err := dynamic.NewForConfig(a.Client.Config)
	if err != nil {
		return false, err
	}
	certificateName := application.ApplicationName + "-tls"
	if endpoint.ManagedCertificateName != "" {
		certificateName = endpoint.ManagedCertificateName
	}
	certificate, err := dynamicClient.Resource(certificateGVR).Namespace(application.Namespace).Get(ctx, certificateName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("读取 Certificate 状态: %w", err)
	}
	conditions, _, _ := unstructured.NestedSlice(certificate.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if ok && condition["type"] == "Ready" && condition["status"] == "True" {
			return true, nil
		}
	}
	return false, nil
}

func ensureManaged(labels map[string]string) error {
	if labels[ManagedByLabel] != ManagedByValue {
		return fmt.Errorf("同名资源不受 Cylism Manager 管理")
	}
	return nil
}
