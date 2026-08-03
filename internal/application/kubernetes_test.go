package application

import (
	"context"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesApplierInspectReleasePodsReportsCrashLoopAfterNormalExit(t *testing.T) {
	labels := map[string]string{ApplicationNameLabel: "browser", ReleaseLabel: "6"}
	clientset := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "browser-abc", Namespace: "dev", Labels: labels},
		Spec:       corev1.PodSpec{NodeName: "worker-a"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "browser", Ready: false, RestartCount: 4,
			State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}},
		}}},
	})
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	runtime, err := applier.InspectReleasePods(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "browser", ReleaseSequence: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Pods) != 1 || runtime.Pods[0].Ready || runtime.Pods[0].Restarts != 4 {
		t.Fatalf("unexpected Pod runtime: %+v", runtime)
	}
	if !strings.Contains(runtime.Diagnostic, "CrashLoopBackOff") || !strings.Contains(runtime.Diagnostic, "正常退出") {
		t.Fatalf("expected normal-exit crash loop diagnostic, got %q", runtime.Diagnostic)
	}
	container := runtime.Pods[0].Containers[0]
	if container.State != "waiting" || container.Reason != "CrashLoopBackOff" || container.LastExitCode == nil || *container.LastExitCode != 0 {
		t.Fatalf("unexpected container runtime: %+v", container)
	}
}

func TestKubernetesApplierInspectReleasePodsReportsReadyPod(t *testing.T) {
	labels := map[string]string{ApplicationNameLabel: "api", ReleaseLabel: "2"}
	clientset := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-abc", Namespace: "dev", Labels: labels},
		Spec:       corev1.PodSpec{NodeName: "worker-a"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "api", Ready: true}}},
	})
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	runtime, err := applier.InspectReleasePods(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "api", ReleaseSequence: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Pods) != 1 || !runtime.Pods[0].Ready || runtime.Diagnostic != "" {
		t.Fatalf("expected ready Pod runtime, got %+v", runtime)
	}
}

func TestKubernetesApplierInspectReleasePodsReportsSchedulingFailure(t *testing.T) {
	labels := map[string]string{ApplicationNameLabel: "api", ReleaseLabel: "3"}
	clientset := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-pending", Namespace: "dev", Labels: labels},
		Status: corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/2 nodes are available: insufficient memory.",
		}}},
	})
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	runtime, err := applier.InspectReleasePods(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "api", ReleaseSequence: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Pods) != 1 || runtime.Pods[0].Ready || !strings.Contains(runtime.Diagnostic, "调度失败") || !strings.Contains(runtime.Diagnostic, "insufficient memory") {
		t.Fatalf("expected scheduling diagnostic, got %+v", runtime)
	}
}

var _ = model.ReleaseRuntime{}

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
	labels := map[string]string{ApplicationNameLabel: "order-api", ReleaseLabel: "2"}
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
	err := applier.WaitReady(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "order-api", ReleaseSequence: 2}, ReleaseSpec{Replicas: 1})
	if err == nil || !strings.Contains(err.Error(), "ImagePullBackOff") || !strings.Contains(err.Error(), "order-api-abc") {
		t.Fatalf("expected image pull diagnostic, got %v", err)
	}
}

func TestKubernetesApplierApplyCreatesStatefulSet(t *testing.T) {
	resources, err := RenderResources(ApplicationContext{ProjectName: "knowledge", EnvironmentName: "production", ApplicationName: "meilisearch", Namespace: "dev", ReleaseSequence: 1, WorkloadKind: WorkloadKindStatefulSet}, validTestReleaseSpec())
	if err != nil {
		t.Fatal(err)
	}
	clientset := k8sfake.NewSimpleClientset()
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	if err := applier.Apply(context.Background(), resources); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.AppsV1().StatefulSets("dev").Get(context.Background(), "meilisearch", metav1.GetOptions{}); err != nil {
		t.Fatalf("expected StatefulSet: %v", err)
	}
	if _, err := clientset.AppsV1().Deployments("dev").Get(context.Background(), "meilisearch", metav1.GetOptions{}); err == nil {
		t.Fatal("StatefulSet release must not create a Deployment")
	}
}

func TestKubernetesApplierWaitReadyIgnoresFailedPodFromOlderRelease(t *testing.T) {
	oldLabels := map[string]string{ApplicationNameLabel: "order-api", ReleaseLabel: "1"}
	currentLabels := map[string]string{ApplicationNameLabel: "order-api", ReleaseLabel: "2"}
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "order-api", Namespace: "dev", Generation: 2}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: 1, AvailableReplicas: 1, UnavailableReplicas: 0}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "order-api-old", Namespace: "dev", Labels: oldLabels}, Status: corev1.PodStatus{Phase: corev1.PodFailed, Message: "old image pull failure"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "order-api-current", Namespace: "dev", Labels: currentLabels}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "order-api", Ready: true}}}},
	)
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	if err := applier.WaitReady(context.Background(), ApplicationContext{Namespace: "dev", ApplicationName: "order-api", ReleaseSequence: 2}, ReleaseSpec{Replicas: 1}); err != nil {
		t.Fatalf("current release must not fail because of an old Pod: %v", err)
	}
}

func TestKubernetesApplierVerifyImageRejectsInvalidReference(t *testing.T) {
	applier := NewKubernetesApplier(nil)
	err := applier.VerifyImage(context.Background(), ReleaseSpec{Image: "invalid image reference"})
	if err == nil || !strings.Contains(err.Error(), "镜像地址无效") {
		t.Fatalf("expected invalid image reference failure, got %v", err)
	}
}

func TestImageVerificationReferenceUsesNodeMirror(t *testing.T) {
	ref, err := imageVerificationReference(ReleaseSpec{
		Image:                     "zenika/alpine-chrome:124",
		ImageVerificationEndpoint: "https://docker.1panel.live",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name() != "docker.1panel.live/zenika/alpine-chrome:124" {
		t.Fatalf("expected node mirror reference, got %q", ref.Name())
	}
}
