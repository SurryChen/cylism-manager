package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestGatewayServiceIsPrivateIdempotentAndChecksOwnership(t *testing.T) {
	ctx := context.Background()
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	config := &model.DatabaseGateway{ID: 8, ListenIP: "100.80.1.1", ListenPort: 13306}
	for range 2 {
		if err := ApplyService(ctx, client, config); err != nil {
			t.Fatal(err)
		}
	}
	service, _ := client.Clientset.CoreV1().Services(namespace).Get(ctx, Name(8), metav1.GetOptions{})
	if service.Spec.Selector != nil || len(service.Spec.ExternalIPs) != 0 || service.Spec.Ports[0].NodePort != 0 {
		t.Fatalf("Service must not expose a public entry: %+v", service.Spec)
	}
	slice, err := client.Clientset.DiscoveryV1().EndpointSlices(namespace).Get(ctx, Name(8), metav1.GetOptions{})
	if err != nil || slice.Endpoints[0].Addresses[0] != config.ListenIP || *slice.Ports[0].Port != 13306 {
		t.Fatalf("incorrect gateway destination: %+v %v", slice, err)
	}
	delete(service.Labels, ownerLabel)
	client.Clientset.CoreV1().Services(namespace).Update(ctx, service, metav1.UpdateOptions{})
	if ApplyService(ctx, client, config) == nil {
		t.Fatal("foreign service was overwritten")
	}
	if DeleteService(ctx, client, config) == nil {
		t.Fatal("foreign service was deleted")
	}
	if _, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, Name(8), metav1.GetOptions{}); err != nil {
		t.Fatal("foreign service must be retained")
	}
}

func TestGatewayProbesNodesAndCleansOnlyItsOwnPods(t *testing.T) {
	ctx := context.Background()
	ready := func(name string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	}
	unschedulable := ready("maintenance")
	unschedulable.Spec.Unschedulable = true
	objects := []runtime.Object{ready("node-a"), ready("node-b"), unschedulable, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "application", Namespace: namespace, Labels: labels(8)}}}
	cs := fake.NewSimpleClientset(objects...)
	client := &k8s.Client{Clientset: cs}
	config := &model.DatabaseGateway{ID: 8, ProbeImage: DefaultProbeImage}
	var created []*corev1.Pod
	cs.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		pod.Name = fmt.Sprintf("probe-%d", len(created))
		pod.GenerateName = ""
		pod.Status.Phase = corev1.PodSucceeded
		created = append(created, pod)
		return true, pod, cs.Tracker().Create(corev1.SchemeGroupVersion.WithResource("pods"), pod, namespace)
	})
	if err := Probe(ctx, client, config, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("expected both schedulable nodes to be checked: %d", len(created))
	}
	for _, pod := range created {
		if pod.Spec.NodeName == "maintenance" || *pod.Spec.AutomountServiceAccountToken || pod.Spec.HostNetwork || !*pod.Spec.SecurityContext.RunAsNonRoot {
			t.Fatalf("unsafe diagnostic pod: %+v", pod.Spec)
		}
		for _, env := range pod.Spec.Containers[0].Env {
			if env.Name != "DB_HOST" {
				t.Fatalf("unexpected diagnostic credentials: %+v", env)
			}
		}
	}
	items, _ := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if len(items.Items) != 1 || items.Items[0].Name != "application" {
		t.Fatalf("probe cleanup changed other workloads: %+v", items.Items)
	}
	stale := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "stale", Namespace: namespace, Labels: labels(8)}}
	stale.Labels[probeLabel] = "true"
	cs.CoreV1().Pods(namespace).Create(ctx, stale, metav1.CreateOptions{})
	if err := CleanupProbes(ctx, client, config); err != nil {
		t.Fatal(err)
	}
	items, _ = cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if len(items.Items) != 1 || items.Items[0].Name != "application" {
		t.Fatalf("recovery cleanup changed other workloads: %+v", items.Items)
	}
}

func TestGatewayFailedProbeRemovesDiagnosticPod(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		pod.Name = "broken-probe"
		pod.GenerateName = ""
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "probe", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}
		return true, pod, cs.Tracker().Create(corev1.SchemeGroupVersion.WithResource("pods"), pod, namespace)
	})
	err := probeNode(context.Background(), &k8s.Client{Clientset: cs}, &model.DatabaseGateway{ID: 1, ProbeImage: DefaultProbeImage}, "node-a")
	if err == nil {
		t.Fatal("failed image pull was reported as successful network check")
	}
	items, _ := cs.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if len(items.Items) != 0 {
		t.Fatal("failed probe pod was not cleaned")
	}
}
