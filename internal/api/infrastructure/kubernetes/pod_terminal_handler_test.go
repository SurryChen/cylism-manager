package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSelectPodTerminalContainer(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}}}

	container, err := selectPodTerminalContainer(pod, "sidecar")
	if err != nil || container != "sidecar" {
		t.Fatalf("expected requested sidecar container, got %q, %v", container, err)
	}
	if _, err := selectPodTerminalContainer(pod, ""); err == nil {
		t.Fatal("expected an error when multiple containers are not disambiguated")
	}
	if _, err := selectPodTerminalContainer(pod, "missing"); err == nil {
		t.Fatal("expected an error for a container not in the Pod")
	}
}

func TestSelectPodTerminalContainerDefaultsSingleContainer(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}}
	container, err := selectPodTerminalContainer(pod, "")
	if err != nil || container != "app" {
		t.Fatalf("expected the only container, got %q, %v", container, err)
	}
}
