package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestRemovalDeletionUsesNodeIdentityAndStatePreconditions(t *testing.T) {
	node := readyNode("worker", true)
	node.UID = "original"
	node.ResourceVersion = "8"
	node.Spec.Unschedulable = true
	node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	cs := k8sfake.NewSimpleClientset(node)
	client := &Client{Clientset: cs}
	cs.PrependReactor("delete", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		options := action.(k8stesting.DeleteAction).GetDeleteOptions()
		if options.Preconditions == nil || *options.Preconditions.UID != node.UID || *options.Preconditions.ResourceVersion != "8" {
			t.Fatalf("unsafe deletion: %#v", options)
		}
		return false, nil, nil
	})
	if err := client.DeleteRemovalNodeContext(context.Background(), "worker", "replacement"); err == nil {
		t.Fatal("wrong UID accepted")
	}
	if err := client.DeleteRemovalNodeContext(context.Background(), "worker", "original"); err != nil {
		t.Fatal(err)
	}
}

func TestRemovalDrainRejectsReplacedNodeAndUsesPodUID(t *testing.T) {
	controller := true
	node := readyNode("worker", true)
	node.UID = "original"
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: types.UID("pod-1"), OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker"}}
	cs := k8sfake.NewSimpleClientset(node, pod)
	client := &Client{Clientset: cs}
	if _, err := client.DrainRemovalNodeContext(context.Background(), "worker", "replacement", DrainOptions{}); err == nil {
		t.Fatal("wrong node UID accepted")
	}
	cs.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		eviction := action.(k8stesting.CreateAction).GetObject().(*policyv1.Eviction)
		if eviction.DeleteOptions == nil || *eviction.DeleteOptions.Preconditions.UID != pod.UID {
			t.Fatalf("missing Pod identity: %#v", eviction)
		}
		return true, nil, apierrors.NewTooManyRequests("PDB blocks eviction", 0)
	})
	result, err := client.DrainRemovalNodeContext(context.Background(), "worker", "original", DrainOptions{})
	if err != nil || len(result.Pending) != 1 {
		t.Fatalf("PDB must remain pending: %#v %v", result, err)
	}
	if err = client.DeleteRemovalNodeContext(context.Background(), "worker", "original"); err == nil {
		t.Fatal("ready/nonempty node deleted")
	}
}

func TestRemovalRejectsControlPlaneEvenWhenDatabaseCallsItWorker(t *testing.T) {
	node := readyNode("worker", false)
	node.UID = "original"
	node.Labels = map[string]string{"node-role.kubernetes.io/control-plane": ""}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(node)}
	if _, err := client.DrainRemovalNodeContext(context.Background(), "worker", "original", DrainOptions{}); err == nil {
		t.Fatal("control plane drain accepted")
	}
	if err := client.DeleteRemovalNodeContext(context.Background(), "worker", "original"); err == nil {
		t.Fatal("control plane deletion accepted")
	}
}
