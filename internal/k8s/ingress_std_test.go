package k8s

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngressStdInfo_FieldsComplete(t *testing.T) {
	i := IngressStdInfo{
		Name:       "web-ingress",
		Namespace:  "default",
		Hosts:      []string{"example.com"},
		Paths:      []string{"/ -> web-svc:80"},
		TLS:        []string{"tls-secret"},
		Controller: "traefik",
		Age:        "3d",
	}
	if i.Controller != "traefik" {
		t.Errorf("expected traefik controller, got %s", i.Controller)
	}
}

func TestIngressControllerStatus_FieldsComplete(t *testing.T) {
	c := IngressControllerStatus{
		Type:      "Traefik",
		Version:   "2.10.7",
		Running:   true,
		CRD:       true,
		Namespace: "kube-system",
	}
	if !c.Running {
		t.Error("expected controller to be running")
	}
}

func TestIngressControllerStatusFromTraefikDeployment(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "traefik", Namespace: "kube-system"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "rancher/mirrored-library-traefik:3.7.4"}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}

	status := ingressControllerStatusFromDeployment(deployment, true)
	if status.Type != "Traefik" || status.Version != "3.7.4" || !status.Running || !status.CRD {
		t.Fatalf("unexpected Traefik status: %+v", status)
	}
	if status.Namespace != "kube-system" {
		t.Fatalf("expected kube-system namespace, got %q", status.Namespace)
	}
}

func TestCachedIngressControllerStatusReturnsFreshCopyOnly(t *testing.T) {
	now := time.Now()
	cached := &IngressControllerStatus{Type: "Traefik", Version: "3.7.4", Running: true}

	status := cachedIngressControllerStatus(cached, now.Add(time.Minute), now)
	if status == nil || status == cached || status.Version != "3.7.4" {
		t.Fatalf("expected a copied fresh cached status, got %+v", status)
	}
	if expired := cachedIngressControllerStatus(cached, now.Add(-time.Second), now); expired != nil {
		t.Fatalf("expected expired cache to be ignored, got %+v", expired)
	}
}

func TestIngressStdMethods_Exist(t *testing.T) {
	client, _ := newClientFromRestConfig(nil)
	_ = client.ListIngresses
	_ = client.GetIngress
	_ = client.CreateIngress
	_ = client.DeleteIngress
	_ = client.DetectIngressController
}
