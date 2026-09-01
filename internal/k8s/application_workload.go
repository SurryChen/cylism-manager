package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var applicationCertificateStatusGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

// ApplicationPreflightAdapter reads cluster objects required before release.
type ApplicationPreflightAdapter struct{ client *Client }

// ApplicationWorkloadControllerAdapter owns Deployment and StatefulSet mutations.
type ApplicationWorkloadControllerAdapter struct{ client *Client }

// ApplicationEndpointAdapter owns Ingress reads and deletion.
type ApplicationEndpointAdapter struct{ client *Client }

// ReleaseDiagnosticsAdapter owns pod, event and certificate status reads.
type ReleaseDiagnosticsAdapter struct{ client *Client }

func NewApplicationPreflightAdapter(client *Client) *ApplicationPreflightAdapter {
	return &ApplicationPreflightAdapter{client: client}
}

func NewApplicationWorkloadControllerAdapter(client *Client) *ApplicationWorkloadControllerAdapter {
	return &ApplicationWorkloadControllerAdapter{client: client}
}

func NewApplicationEndpointAdapter(client *Client) *ApplicationEndpointAdapter {
	return &ApplicationEndpointAdapter{client: client}
}

func NewReleaseDiagnosticsAdapter(client *Client) *ReleaseDiagnosticsAdapter {
	return &ReleaseDiagnosticsAdapter{client: client}
}

func (a *ApplicationWorkloadControllerAdapter) GetDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	return a.client.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationWorkloadControllerAdapter) UpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return a.client.Clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
}
func (a *ApplicationWorkloadControllerAdapter) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	return a.client.Clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationWorkloadControllerAdapter) UpdateStatefulSet(ctx context.Context, workload *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	return a.client.Clientset.AppsV1().StatefulSets(workload.Namespace).Update(ctx, workload, metav1.UpdateOptions{})
}

func (a *ReleaseDiagnosticsAdapter) ListPods(ctx context.Context, ns, selector string) ([]corev1.Pod, error) {
	list, err := a.client.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
func (a *ReleaseDiagnosticsAdapter) ListEvents(ctx context.Context, ns string) ([]corev1.Event, error) {
	list, err := a.client.Clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
func (a *ApplicationEndpointAdapter) DeleteIngress(ctx context.Context, ns, name string) error {
	return a.client.Clientset.NetworkingV1().Ingresses(ns).Delete(ctx, name, metav1.DeleteOptions{})
}
func (a *ApplicationEndpointAdapter) GetIngress(ctx context.Context, ns, name string) (*networkingv1.Ingress, error) {
	return a.client.Clientset.NetworkingV1().Ingresses(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationPreflightAdapter) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	return a.client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationPreflightAdapter) GetSecret(ctx context.Context, ns, name string) (*corev1.Secret, error) {
	return a.client.Clientset.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationPreflightAdapter) GetConfigMap(ctx context.Context, ns, name string) (*corev1.ConfigMap, error) {
	return a.client.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
}
func (a *ApplicationPreflightAdapter) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	return a.client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}
func (a *ReleaseDiagnosticsAdapter) CertificateReady(ctx context.Context, namespace, name string) (bool, error) {
	if a == nil || a.client == nil {
		return false, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	dynamicClient, err := a.client.dynamicClient()
	if err != nil {
		return false, err
	}
	certificate, err := dynamicClient.Resource(applicationCertificateStatusGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
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
func (a *ApplicationPreflightAdapter) DetectIngressController(ctx context.Context) (*IngressControllerStatus, error) {
	return a.client.DetectIngressControllerContext(ctx)
}
func (a *ApplicationPreflightAdapter) CheckCRD(ctx context.Context, name string) (bool, error) {
	return a.client.CheckCRDContext(ctx, name)
}
func (a *ApplicationPreflightAdapter) GetManagedPVC(ctx context.Context, ns, name string, id uint) (*PersistentVolumeClaimInfo, error) {
	return a.client.GetManagedPVCContext(ctx, ns, name, id)
}
