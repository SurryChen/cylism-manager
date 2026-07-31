package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDrainPlanSeparatesSafePodsFromBlockers(t *testing.T) {
	controller := true
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		readyNode("worker-a", false),
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a", Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "default"}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
	)}

	plan, err := client.DrainPlan("worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Evictable) != 1 || plan.Evictable[0].Name != "web" || len(plan.RequiresEmptyDirConfirmation) != 1 || len(plan.Blocked) != 1 || len(plan.Skipped) != 1 {
		t.Fatalf("unexpected drain plan: %#v", plan)
	}
}

func TestDeleteNodeRequiresStoppedAndDrainedNode(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(readyNode("worker-a", true))}
	if err := client.DeleteNode("worker-a"); err == nil {
		t.Fatal("expected a ready node to be rejected")
	}
}

func TestDrainNodeUsesEvictionInsteadOfDeletingPods(t *testing.T) {
	controller := true
	clientset := k8sfake.NewSimpleClientset(
		readyNode("worker-a", false),
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
	)
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			return true, nil, nil
		}
		return false, nil, nil
	})
	client := &Client{Clientset: clientset}

	result, err := client.DrainNode("worker-a", DrainOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evicted) != 1 || result.Evicted[0].Name != "web" {
		t.Fatalf("expected one eviction, got %#v", result)
	}
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			t.Fatalf("drain must not delete pods directly: %#v", action)
		}
	}
}

func readyNode(name string, cordoned bool) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: corev1.NodeSpec{Unschedulable: cordoned}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
}
