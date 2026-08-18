package application

import (
	"context"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

func TestKubernetesApplierPreflightValidatesFileMountSources(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "edge-tls", Namespace: "dev"}, Data: map[string][]byte{"tls.crt": []byte("cert")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "edge-config", Namespace: "dev"}, Data: map[string]string{"config.yaml": "port: 443"}},
	)
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	application := ApplicationContext{Namespace: "dev"}
	spec := ReleaseSpec{Endpoint: EndpointSpec{Exposure: ExposureCluster}, FileMounts: []FileMountSpec{
		{SourceType: FileMountSourceSecret, SourceName: "edge-tls", Key: "tls.crt", MountPath: "/run/tls/tls.crt"},
		{SourceType: FileMountSourceConfigMap, SourceName: "edge-config", Key: "config.yaml", MountPath: "/etc/edge/config.yaml"},
	}}
	if err := applier.Preflight(context.Background(), application, spec); err != nil {
		t.Fatalf("expected file mount source preflight to pass: %v", err)
	}
	spec.FileMounts[0].Key = "tls.key"
	if err := applier.Preflight(context.Background(), application, spec); err == nil || !strings.Contains(err.Error(), "tls.key") {
		t.Fatalf("expected missing Secret key failure, got %v", err)
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

func TestKubernetesApplierSyncApplicationEndpointsMergesHostsAndTLSSecrets(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	applicationContext := ApplicationContext{ProjectID: 1, EnvironmentID: 2, Namespace: "dev", ApplicationName: "order-api"}
	endpoints := []model.ApplicationEndpoint{
		{Domain: "api.example.com", Path: "/", TLSEnabled: true, TLSSecretName: "api-tls"},
		{Domain: "admin.example.com", Path: "/console", TLSEnabled: true, TLSSecretName: "admin-tls"},
		{Domain: "docs.example.com", Path: "/", TLSEnabled: true, TLSSecretName: "api-tls"},
		{Domain: "plain.example.com", Path: "/", TLSEnabled: false},
	}
	if err := applier.SyncApplicationEndpoints(context.Background(), applicationContext, endpoints, 8080); err != nil {
		t.Fatal(err)
	}
	ingress, err := clientset.NetworkingV1().Ingresses("dev").Get(context.Background(), "order-api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ingress.Spec.Rules) != 4 || ingress.Spec.Rules[1].Host != "admin.example.com" || ingress.Spec.Rules[1].HTTP.Paths[0].Path != "/console" {
		t.Fatalf("unexpected ingress rules: %#v", ingress.Spec.Rules)
	}
	if len(ingress.Spec.TLS) != 2 || ingress.Spec.TLS[0].SecretName != "api-tls" || len(ingress.Spec.TLS[0].Hosts) != 2 || ingress.Spec.TLS[1].SecretName != "admin-tls" {
		t.Fatalf("expected grouped TLS entries, got %#v", ingress.Spec.TLS)
	}
	if ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number != 8080 {
		t.Fatalf("unexpected service port: %#v", ingress.Spec.Rules[0])
	}
}

func TestKubernetesApplierSyncApplicationEndpointsDeletesOnlyManagedIngress(t *testing.T) {
	applicationContext := ApplicationContext{ProjectID: 1, EnvironmentID: 2, Namespace: "dev", ApplicationName: "order-api"}
	managed := applicationEndpointsIngress(applicationContext, []model.ApplicationEndpoint{{Domain: "api.example.com", Path: "/"}}, 80)
	clientset := k8sfake.NewSimpleClientset(managed)
	applier := NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	if err := applier.SyncApplicationEndpoints(context.Background(), applicationContext, nil, 80); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.NetworkingV1().Ingresses("dev").Get(context.Background(), "order-api", metav1.GetOptions{}); err == nil {
		t.Fatal("expected managed ingress to be removed after last endpoint deletion")
	}

	unmanaged := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "order-api", Namespace: "dev"}}
	clientset = k8sfake.NewSimpleClientset(unmanaged)
	applier = NewKubernetesApplier(&k8sclient.Client{Clientset: clientset})
	if err := applier.SyncApplicationEndpoints(context.Background(), applicationContext, nil, 80); err == nil || !strings.Contains(err.Error(), "不受 Cylism Manager 管理") {
		t.Fatalf("expected unmanaged ingress protection, got %v", err)
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
