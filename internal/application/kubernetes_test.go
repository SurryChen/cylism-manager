package application

import (
	"context"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesApplierPreflightRequiresActiveNamespace(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	application := ApplicationContext{Namespace: "dev"}
	if err := applier.Preflight(context.Background(), application, ReleaseSpec{Endpoint: EndpointSpec{Exposure: ExposureCluster}}); err == nil || !strings.Contains(err.Error(), "命名空间") {
		t.Fatalf("expected missing namespace preflight failure, got %v", err)
	}
	if _, err := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := applier.Preflight(context.Background(), application, ReleaseSpec{Endpoint: EndpointSpec{Exposure: ExposureCluster}}); err != nil {
		t.Fatalf("expected active namespace preflight to pass: %v", err)
	}
}

func TestKubernetesApplierWaitReadyReportsImagePullFailure(t *testing.T) {
	labels := map[string]string{ApplicationNameLabel: "order-api"}
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "order-api", Namespace: "dev"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: labels}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "order-api-abc", Namespace: "dev", Labels: labels},
			Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "order-api",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "failed to pull image registry.example.com/order-api:1.2.3"}},
			}}},
		},
	)
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	err := applier.WaitReady(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "order-api"}, ReleaseSpec{Replicas: 1})
	if err == nil || !strings.Contains(err.Error(), "ImagePullBackOff") || !strings.Contains(err.Error(), "order-api-abc") {
		t.Fatalf("expected image pull diagnostic, got %v", err)
	}
}

func TestKubernetesApplierVerifyImageRejectsInvalidReference(t *testing.T) {
	applier := NewKubernetesApplier(nil)
	err := applier.VerifyImage(context.Background(), ReleaseSpec{Image: "invalid image reference"})
	if err == nil || !strings.Contains(err.Error(), "镜像地址无效") {
		t.Fatalf("expected invalid image reference failure, got %v", err)
	}
}
