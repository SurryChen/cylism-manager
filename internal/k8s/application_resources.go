package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var applicationCertificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

// ApplicationResourceApplier is the Kubernetes mutation adapter used by the
// application release service. It intentionally exposes only resources that
// are rendered by an application release.
type ApplicationResourceApplier struct {
	client *Client
}

func NewApplicationResourceApplier(client *Client) *ApplicationResourceApplier {
	return &ApplicationResourceApplier{client: client}
}

func (a *ApplicationResourceApplier) ApplyConfigMap(ctx context.Context, desired *corev1.ConfigMap) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.client.Clientset.CoreV1().ConfigMaps(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplySecret(ctx context.Context, desired *corev1.Secret) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.CoreV1().Secrets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.CoreV1().Secrets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.client.Clientset.CoreV1().Secrets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplyDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.AppsV1().Deployments(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.AppsV1().Deployments(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.client.Clientset.AppsV1().Deployments(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplyStatefulSet(ctx context.Context, desired *appsv1.StatefulSet) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.AppsV1().StatefulSets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.AppsV1().StatefulSets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.client.Clientset.AppsV1().StatefulSets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplyService(ctx context.Context, desired *corev1.Service) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.CoreV1().Services(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.CoreV1().Services(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	_, err = a.client.Clientset.CoreV1().Services(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplyIngress(ctx context.Context, desired *networkingv1.Ingress) error {
	if err := a.ready(); err != nil {
		return err
	}
	existing, err := a.client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.Labels); err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = a.client.Clientset.NetworkingV1().Ingresses(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ApplyCertificate(ctx context.Context, desired *unstructured.Unstructured) error {
	if err := a.ready(); err != nil {
		return err
	}
	dynamicClient, err := a.client.dynamicClient()
	if err != nil {
		return err
	}
	resource := dynamicClient.Resource(applicationCertificateGVR).Namespace(desired.GetNamespace())
	existing, err := resource.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureApplicationManaged(existing.GetLabels()); err != nil {
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (a *ApplicationResourceApplier) ready() error {
	if a == nil || a.client == nil || a.client.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	return nil
}

func ensureApplicationManaged(labels map[string]string) error {
	if labels[ManagedByLabel] != ManagedByValue {
		return fmt.Errorf("同名资源不受 Cylism Manager 管理")
	}
	return nil
}
