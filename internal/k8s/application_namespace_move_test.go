package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStageApplicationNamespacePreservesSourceAndChecksReadyImage(t *testing.T) {
	move := ApplicationNamespaceMove{ApplicationID: 1, SourceSequence: 2, SourceProjectID: 3, TargetProjectID: 4, TargetEnvironmentID: 5, Name: "account", SourceNamespace: "old", TargetNamespace: "new"}
	labels := map[string]string{ManagedByLabel: ManagedByValue, "app.kubernetes.io/part-of": "project-3", "app": "account", "cylism.io/release": "2"}
	selector := map[string]string{"app": "account"}
	replicas, yes := int32(1), true
	ready := appsv1.DeploymentStatus{UpdatedReplicas: 1, AvailableReplicas: 1}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: "account", UID: "origin", Labels: labels},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "account", Image: "account:v1", ReadinessProbe: &corev1.Probe{},
			}}}}},
		Status: ready,
	}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "old", Name: "account", UID: "source-service", Labels: labels},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: selector}}
	pod := func(ns string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "pod", Labels: labels},
			Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "account", Ready: true, ImageID: "image@sha256:abc"}}}}
	}
	clientset := fake.NewSimpleClientset(deployment, service, pod("old"), pod("new"))
	client := &Client{Clientset: clientset}
	ctx := t.Context()
	if status, err := client.StageApplicationNamespace(ctx, move, true); err != nil || !status.Prepared || status.Ready {
		t.Fatalf("prepare: %+v, %v", status, err)
	}
	target, err := clientset.AppsV1().Deployments("new").Get(ctx, "account", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	target.Status = ready
	if _, err := clientset.AppsV1().Deployments("new").UpdateStatus(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Namespace: "new", Name: "account", Labels: map[string]string{discoveryv1.LabelServiceName: "account"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Service", UID: types.UID(""), Controller: &yes}}},
		Endpoints: []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: &yes}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "new", Name: "pod"}}},
	}
	if _, err := clientset.DiscoveryV1().EndpointSlices("new").Create(ctx, slice, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, err := client.StageApplicationNamespace(ctx, move, true); err != nil || !status.Ready {
		t.Fatalf("resume: %+v, %v", status, err)
	}
	actual, err := clientset.CoreV1().Pods("new").Get(ctx, "pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	actual.Status.ContainerStatuses[0].ImageID = "image@sha256:changed"
	if _, err := clientset.CoreV1().Pods("new").UpdateStatus(ctx, actual, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, err := client.StageApplicationNamespace(ctx, move, false); err == nil || status.Ready {
		t.Fatalf("accepted a changed digest: %+v, %v", status, err)
	}
	for _, action := range clientset.Actions() {
		if action.GetNamespace() == "old" && action.GetVerb() != "get" && action.GetVerb() != "list" {
			t.Fatalf("modified source namespace: %+v", action)
		}
	}
}
