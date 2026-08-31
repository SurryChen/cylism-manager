package application

import (
	"context"
	"testing"

	applicationdomain "github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestQueryServiceApplicationRuntimeInfosAssemblesWorkloadsAndServices(t *testing.T) {
	replicas := int32(2)
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)}}},
			Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.20"}}}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 2, AvailableReplicas: 2},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "demo"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
			Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1, AvailableReplicas: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "demo", Labels: map[string]string{applicationdomain.ApplicationNameLabel: "api"}},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "api", Ready: true}}},
		},
	)}
	applications := []model.Application{
		{ID: 1, Name: "api", Environment: model.Environment{Namespace: "demo"}, WorkloadKind: applicationdomain.WorkloadKindDeployment},
		{ID: 2, Name: "database", Environment: model.Environment{Namespace: "demo"}, WorkloadKind: applicationdomain.WorkloadKindStatefulSet},
	}
	runtimes := NewQueryService(queryStoreStub{}).ApplicationRuntimeInfos(context.Background(), client, applications, map[uint][]model.Release{
		1: {{ID: 11, Sequence: 2, Version: "1.2.0", Status: model.ReleaseStatusSucceeded}},
		2: {{ID: 12, Sequence: 1, Version: "1.0.0", Status: model.ReleaseStatusSucceeded}},
	})

	api := runtimes[1]
	if api.Status != "running" || api.Service == nil || api.Service.Ports[0].TargetPort != "8080" || api.LatestRelease == nil || api.LatestRelease.Sequence != 2 {
		t.Fatalf("unexpected API runtime: %#v", api)
	}
	if len(api.ReadyPods) != 1 || api.ReadyPods[0].NodeName != "node-a" || len(api.ReadyNodes) != 1 || api.ReadyNodes[0] != "node-a" {
		t.Fatalf("unexpected ready pod information: %#v", api)
	}
	database := runtimes[2]
	if database.Status != "degraded" || database.DesiredReplicas != 2 || database.ReadyReplicas != 1 {
		t.Fatalf("unexpected StatefulSet runtime: %#v", database)
	}
}

func TestQueryServiceWorkspacePodStatesCountsReadyPods(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		workspacePod("api-ready", "demo", "api", "4", corev1.PodRunning, true),
		workspacePod("api-pending", "demo", "api", "4", corev1.PodPending, false),
		workspacePod("ignored", "demo", "api", "invalid", corev1.PodRunning, true),
	)}
	states, available := NewQueryService(queryStoreStub{}).WorkspacePodStates(context.Background(), client, "demo")
	if !available {
		t.Fatal("WorkspacePodStates() availability = false")
	}
	state := states[WorkspacePodKey("api", 4)]
	if state.Status != "degraded" || state.ReadyPods != 1 || state.TotalPods != 2 {
		t.Fatalf("unexpected pod state: %#v", state)
	}
	states, available = NewQueryService(queryStoreStub{}).WorkspacePodStates(context.Background(), nil, "demo")
	if available || len(states) != 0 {
		t.Fatalf("unavailable client result = %#v, %t", states, available)
	}
}

func workspacePod(name, namespace, applicationName, release string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{
			applicationdomain.ManagedByLabel:       applicationdomain.ManagedByValue,
			applicationdomain.ApplicationNameLabel: applicationName,
			applicationdomain.ReleaseLabel:         release,
		}},
		Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: ready}}},
	}
}
