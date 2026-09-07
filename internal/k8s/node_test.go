package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	plan, err := client.DrainPlanContext(context.Background(), "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Evictable) != 1 || plan.Evictable[0].Name != "web" || len(plan.RequiresEmptyDirConfirmation) != 1 || len(plan.Blocked) != 1 || len(plan.Skipped) != 1 {
		t.Fatalf("unexpected drain plan: %#v", plan)
	}
}

func TestDeleteNodeRequiresStoppedAndDrainedNode(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(readyNode("worker-a", true))}
	if err := client.DeleteNodeContext(context.Background(), "worker-a"); err == nil {
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

	result, err := client.DrainNodeContext(context.Background(), "worker-a", DrainOptions{})
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

func TestNodeHealthStateDistinguishesTransientNotReadyFromFailed(t *testing.T) {
	now := time.Now()
	ready := nodeToInfo(readyNode("ready", false))
	if ready.HealthState != NodeHealthReady {
		t.Fatalf("expected ready state, got %#v", ready)
	}

	notReadyNode := readyNode("restarting", false)
	notReadyNode.Status.Conditions[0] = corev1.NodeCondition{
		Type:              corev1.NodeReady,
		Status:            corev1.ConditionFalse,
		Reason:            "KubeletNotReady",
		LastHeartbeatTime: metav1.NewTime(now.Add(-time.Minute)),
	}
	notReady := nodeToInfo(notReadyNode)
	if notReady.HealthState != NodeHealthNotReady {
		t.Fatalf("expected transient not-ready state, got %#v", notReady)
	}

	failedNode := readyNode("failed", false)
	failedNode.Status.Conditions[0] = corev1.NodeCondition{
		Type:              corev1.NodeReady,
		Status:            corev1.ConditionUnknown,
		Reason:            "NodeStatusUnknown",
		LastHeartbeatTime: metav1.NewTime(now.Add(-nodeFailureThreshold - time.Minute)),
	}
	failed := nodeToInfo(failedNode)
	if failed.HealthState != NodeHealthFailed || failed.LastHeartbeatAt == "" {
		t.Fatalf("expected failed state with heartbeat, got %#v", failed)
	}
}

func TestListNodesContextReturnsRawNodes(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(readyNode("worker-a", false))}
	nodes, err := client.ListNodesContext(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].Name != "worker-a" {
		t.Fatalf("unexpected node list: %#v %v", nodes, err)
	}
	if _, err := (&Client{}).ListNodesContext(context.Background()); err == nil {
		t.Fatal("expected uninitialized client error")
	}
}

func TestNodeInfoMarksCordonedNodeAsEvicted(t *testing.T) {
	info := nodeToInfo(readyNode("worker-a", true))
	if !info.Evicted {
		t.Fatalf("expected cordoned node to be marked evicted, got %#v", info)
	}
}

func TestUpdateNodeLabelsOnlyChangesValidCustomLabels(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Labels: map[string]string{
		corev1.LabelHostname:                    "worker-a",
		"node-role.kubernetes.io/control-plane": "true",
		"cylism.io/zone":                        "old",
	}}})
	client := &Client{Clientset: clientset}

	labels, err := client.UpdateNodeLabelsContext(context.Background(), "worker-a", map[string]string{"cylism.io/zone": "shanghai", "team": "platform"}, []string{"cylism.io/unused"})
	if err != nil {
		t.Fatal(err)
	}
	if labels.Labels["cylism.io/zone"] != "shanghai" || labels.Labels["team"] != "platform" || labels.Labels[corev1.LabelHostname] != "worker-a" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestUpdateNodeLabelsRejectsProtectedAndInvalidLabels(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Labels: map[string]string{corev1.LabelHostname: "worker-a"}}})}
	for name, update := range map[string]struct {
		set    map[string]string
		remove []string
	}{
		"protected set":     {set: map[string]string{"k3s.io/internal-ip": "10.0.0.1"}},
		"protected removal": {remove: []string{corev1.LabelHostname}},
		"invalid key":       {set: map[string]string{"not valid": "value"}},
		"invalid value":     {set: map[string]string{"team": "not valid"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := client.UpdateNodeLabelsContext(context.Background(), "worker-a", update.set, update.remove); err == nil {
				t.Fatal("expected label update to be rejected")
			}
		})
	}
}

func TestRejoinNodeUncordonsNodeAndClearsDrainMarker(t *testing.T) {
	node := readyNode("worker-a", true)
	node.Annotations = map[string]string{nodeDrainAnnotation: "2026-08-01T00:00:00Z"}
	clientset := k8sfake.NewSimpleClientset(node)
	client := &Client{Clientset: clientset}

	info, err := client.RejoinNodeContext(context.Background(), "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if info.Evicted {
		t.Fatalf("expected rejoined node to be schedulable, got %#v", info)
	}
	updated, err := clientset.CoreV1().Nodes().Get(context.Background(), "worker-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Unschedulable || updated.Annotations[nodeDrainAnnotation] != "" {
		t.Fatalf("expected node to be uncordoned and marker cleared, got %#v", updated)
	}
}

func TestForceDrainDeletesOnlyControlledPodsOnFailedWorker(t *testing.T) {
	controller := true
	failed := failedWorkerNode("worker-a")
	clientset := k8sfake.NewSimpleClientset(
		failed,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "daemon", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "static", Namespace: "default", Annotations: map[string]string{mirrorPodAnnotation: "hash"}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "default"}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
	)
	client := &Client{Clientset: clientset}

	result, err := client.ForceDrainNodeContext(context.Background(), "worker-a", ForceDrainOptions{AcknowledgeRisk: true, ConfirmNodeName: "worker-a", DeleteEmptyDirData: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Forced || len(result.Deleted) != 1 || result.Deleted[0].Name != "web" || len(result.Blocked) != 1 || result.Blocked[0].Name != "debug" || len(result.Skipped) != 2 {
		t.Fatalf("unexpected force drain result: %#v", result)
	}
	updated, err := clientset.CoreV1().Nodes().Get(context.Background(), "worker-a", metav1.GetOptions{})
	if err != nil || !updated.Spec.Unschedulable {
		t.Fatalf("expected failed worker to be cordoned: %#v, %v", updated, err)
	}
	for _, action := range clientset.Actions() {
		if action.GetSubresource() == "eviction" {
			t.Fatalf("force drain must not use eviction: %#v", action)
		}
	}
}

func TestForceDrainRejectsUnsafeRequests(t *testing.T) {
	for name, test := range map[string]struct {
		node    *corev1.Node
		options ForceDrainOptions
	}{
		"healthy worker":          {node: readyNode("worker-a", false), options: ForceDrainOptions{AcknowledgeRisk: true, ConfirmNodeName: "worker-a"}},
		"missing acknowledgement": {node: failedWorkerNode("worker-a"), options: ForceDrainOptions{ConfirmNodeName: "worker-a"}},
		"wrong confirmation":      {node: failedWorkerNode("worker-a"), options: ForceDrainOptions{AcknowledgeRisk: true, ConfirmNodeName: "other"}},
		"control plane":           {node: failedControlPlaneNode("server-a"), options: ForceDrainOptions{AcknowledgeRisk: true, ConfirmNodeName: "server-a"}},
	} {
		t.Run(name, func(t *testing.T) {
			client := &Client{Clientset: k8sfake.NewSimpleClientset(test.node)}
			if _, err := client.ForceDrainNodeContext(context.Background(), test.node.Name, test.options); err == nil {
				t.Fatal("expected unsafe force drain to be rejected")
			}
		})
	}
}

func TestDrainNodeOnlyLabelsPDBViolationsAsPDB(t *testing.T) {
	controller := true
	for name, test := range map[string]struct {
		evictionError error
		wantPrefix    string
	}{
		"pdb":        {evictionError: apierrors.NewTooManyRequests("Cannot evict pod as it would violate the pod's disruption budget.", 0), wantPrefix: "PodDisruptionBudget 暂不允许驱逐"},
		"rate limit": {evictionError: apierrors.NewTooManyRequests("request throttled", 0), wantPrefix: "提交驱逐请求失败"},
	} {
		t.Run(name, func(t *testing.T) {
			clientset := k8sfake.NewSimpleClientset(
				readyNode("worker-a", false),
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "worker-a"}},
			)
			clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() == "eviction" {
					return true, nil, test.evictionError
				}
				return false, nil, nil
			})
			result, err := (&Client{Clientset: clientset}).DrainNodeContext(context.Background(), "worker-a", DrainOptions{})
			if err != nil || len(result.Pending) != 1 || !strings.HasPrefix(result.Pending[0].Reason, test.wantPrefix) {
				t.Fatalf("unexpected drain result: %#v, %v", result, err)
			}
		})
	}
}

func readyNode(name string, cordoned bool) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: corev1.NodeSpec{Unschedulable: cordoned}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
}

func failedWorkerNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "NodeStatusUnknown", LastHeartbeatTime: metav1.NewTime(time.Now().Add(-nodeFailureThreshold - time.Minute))}}}}
}

func failedControlPlaneNode(name string) *corev1.Node {
	node := failedWorkerNode(name)
	node.Labels = map[string]string{"node-role.kubernetes.io/control-plane": "true"}
	return node
}
