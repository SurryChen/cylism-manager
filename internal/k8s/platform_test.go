package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestUpdatePlatformDeploymentOnlyUpdatesPlatformContainer(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: platformDeploymentName, Namespace: platformDeploymentNamespace},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "sidecar", Image: "example.com/sidecar:1"},
			{Name: platformContainerName, Image: "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}}}},
	})}

	previous, err := client.UpdatePlatformDeployment("registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 9)
	if err != nil {
		t.Fatal(err)
	}
	if previous != "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected previous image: %q", previous)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(t.Context(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != "example.com/sidecar:1" || deployment.Spec.Template.Spec.Containers[1].Image != "registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || deployment.Spec.Template.Annotations[platformReleaseAnnotation] != "9" {
		t.Fatalf("unexpected deployment update: %#v", deployment.Spec.Template)
	}
}

func TestUpdatePlatformDeploymentRejectsMissingPlatformContainer(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: platformDeploymentName, Namespace: platformDeploymentNamespace}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "other", Image: "example.com/other"}}}}}})}
	if _, err := client.UpdatePlatformDeployment("registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 9); err == nil {
		t.Fatal("expected missing platform container rejection")
	}
}
