package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApplyCreatesRuntimeResourcesAndReusesPVC(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 7, Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot:latest", Namespace: DefaultNamespace, PVCName: "nanobot-main-data", Storage: "10Gi", ModelName: "gpt-test", ModelBaseURL: "https://provider.example/v1", APIStyle: ModelProtocolResponses}
	if err := manager.Apply(context.Background(), instance, "model-secret", "runtime-secret"); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if instance.EndpointURL == "" || instance.SecretName == "" {
		t.Fatalf("expected endpoint and secret name, got %#v", instance)
	}
	if _, err := client.Clientset.CoreV1().Namespaces().Get(context.Background(), DefaultNamespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(DefaultNamespace).Get(context.Background(), instance.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment not created: %v", err)
	}
	pod := deployment.Spec.Template.Spec
	if len(pod.InitContainers) != 2 || len(pod.Containers) != 3 || pod.InitContainers[0].Name != PermissionFixInit || pod.InitContainers[1].Name != "render-config" || pod.Containers[0].Name != "gateway" || pod.Containers[1].Name != "api" || pod.Containers[2].Name != "session-api" {
		t.Fatalf("unexpected Nanobot pod: %#v", pod)
	}
	fixPerms := pod.InitContainers[0]
	if fixPerms.SecurityContext == nil || fixPerms.SecurityContext.RunAsUser == nil || *fixPerms.SecurityContext.RunAsUser != 0 || fixPerms.SecurityContext.RunAsNonRoot == nil || *fixPerms.SecurityContext.RunAsNonRoot {
		t.Fatalf("fix-perms init must run as root only: %#v", fixPerms.SecurityContext)
	}
	if fixPerms.SecurityContext.Capabilities == nil || len(fixPerms.SecurityContext.Capabilities.Add) != 2 || fixPerms.SecurityContext.Capabilities.Add[0] != "CHOWN" || fixPerms.SecurityContext.Capabilities.Add[1] != "DAC_READ_SEARCH" {
		t.Fatalf("fix-perms init must only add CAP_CHOWN and CAP_DAC_READ_SEARCH: %#v", fixPerms.SecurityContext.Capabilities)
	}
	if len(fixPerms.Args) != 1 || !strings.Contains(fixPerms.Args[0], "chown -R 1000:1000 /data") {
		t.Fatalf("fix-perms init must chown the workspace to uid 1000: %#v", fixPerms)
	}
	if len(fixPerms.VolumeMounts) != 1 || fixPerms.VolumeMounts[0].Name != "data" || fixPerms.VolumeMounts[0].MountPath != "/data" {
		t.Fatalf("fix-perms init must mount the runtime PVC at /data: %#v", fixPerms.VolumeMounts)
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken || pod.SecurityContext == nil || pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot {
		t.Fatalf("expected restrictive pod security context: %#v", pod)
	}
	if pod.Containers[1].ReadinessProbe == nil || pod.Containers[1].ReadinessProbe.HTTPGet.Port.IntVal != 8900 {
		t.Fatalf("API readiness probe must use port 8900: %#v", pod.Containers[1].ReadinessProbe)
	}
	service, err := client.Clientset.CoreV1().Services(DefaultNamespace).Get(context.Background(), instance.Name, metav1.GetOptions{})
	if err != nil || len(service.Spec.Ports) != 2 || service.Spec.Ports[0].Port != 8900 || service.Spec.Ports[0].TargetPort.IntVal != 8900 || service.Spec.Ports[1].Port != 18800 || service.Spec.Ports[1].TargetPort.IntVal != 18800 {
		t.Fatalf("unexpected Runtime service: %v %#v", err, service)
	}
	secret, err := client.Clientset.CoreV1().Secrets(DefaultNamespace).Get(context.Background(), instance.SecretName, metav1.GetOptions{})
	if err != nil || secret.StringData[RuntimeSecretKey] != "model-secret" || secret.StringData[RuntimeAPISecretKey] != "runtime-secret" {
		t.Fatalf("expected both credential keys in secret: %v %#v", err, secret)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(DefaultNamespace).Get(context.Background(), instance.PVCName, metav1.GetOptions{}); err != nil {
		t.Fatalf("pvc not created: %v", err)
	}
	if err := manager.Apply(context.Background(), instance, "model-secret-2", "runtime-secret-2"); err != nil {
		t.Fatalf("reapply runtime: %v", err)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(DefaultNamespace).Get(context.Background(), instance.PVCName, metav1.GetOptions{})
	if err != nil || claim.Spec.Resources.Requests.Storage().String() != "10Gi" {
		t.Fatalf("unexpected pvc after reapply: %v %#v", err, claim)
	}
}

func TestApplyRejectsForeignPVC(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: DefaultNamespace}})}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 1, Name: "nanobot", RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot", Namespace: DefaultNamespace, PVCName: "data", Storage: "1Gi"}
	if err := manager.Apply(context.Background(), instance, "", ""); err == nil {
		t.Fatal("expected foreign PVC to be rejected")
	}
}

func TestApplyInjectsRestrictedCLIInstallerOnlyWhenAgentToolIsEnabled(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 8, Name: "nanobot-agent", RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot:latest", Namespace: DefaultNamespace, PVCName: "nanobot-agent-data", Storage: "1Gi", ModelName: "gpt-test", ModelBaseURL: "https://provider.example/v1", APIStyle: ModelProtocolResponses, AgentToolEnabled: true}
	if err := manager.Apply(context.Background(), instance, "model-secret", "runtime-secret"); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	serviceAccount, err := client.Clientset.CoreV1().ServiceAccounts(DefaultNamespace).Get(context.Background(), RuntimeAgentServiceAccountName(instance), metav1.GetOptions{})
	if err != nil || serviceAccount.Labels[RuntimeIDLabel] != "8" {
		t.Fatalf("expected Runtime-specific service account: %v %#v", err, serviceAccount)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(DefaultNamespace).Get(context.Background(), instance.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	pod := deployment.Spec.Template.Spec
	if pod.ServiceAccountName != RuntimeAgentServiceAccountName(instance) || pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatalf("expected explicit non-automounted service account: %#v", pod)
	}
	if volumeByName(pod.Volumes, RuntimeCLIVolumeName) == nil || projectedAudience(volumeByName(pod.Volumes, RuntimeInstallerTokenVolumeName)) != RuntimeInstallerTokenAudience || projectedAudience(volumeByName(pod.Volumes, RuntimeAgentTokenVolumeName)) != RuntimeAgentTokenAudience {
		t.Fatalf("expected CLI emptyDir and separated projected identities: %#v", pod.Volumes)
	}
	installer := containerByName(pod.InitContainers, RuntimeCLIInstallerName)
	if installer == nil || len(installer.Command) != 1 || installer.Command[0] != "cylism-install-cli" || installer.SecurityContext == nil || installer.SecurityContext.RunAsNonRoot == nil || !*installer.SecurityContext.RunAsNonRoot {
		t.Fatalf("expected non-root CLI installer: %#v", installer)
	}
	if !hasMount(installer.VolumeMounts, RuntimeCLIVolumeName, RuntimeCLIMountPath, false) || !hasMount(installer.VolumeMounts, RuntimeInstallerTokenVolumeName, RuntimeInstallerTokenMountPath, true) {
		t.Fatalf("installer must receive only CLI target and installer token: %#v", installer.VolumeMounts)
	}
	for _, name := range []string{"gateway", "api"} {
		container := containerByName(pod.Containers, name)
		if container == nil || !hasMount(container.VolumeMounts, RuntimeCLIVolumeName, RuntimeCLIMountPath, true) || !hasMount(container.VolumeMounts, RuntimeAgentTokenVolumeName, RuntimeAgentTokenMountPath, true) || !hasEnv(container.Env, "CYLISM_PLATFORM_TOOL_ENABLED", "true") || !hasEnv(container.Env, "CYLISM_AGENT_TOKEN_FILE", RuntimeAgentTokenFile) {
			t.Fatalf("expected platform capability wiring on %s: %#v", name, container)
		}
	}
	sessionAPI := containerByName(pod.Containers, "session-api")
	if sessionAPI == nil || hasMount(sessionAPI.VolumeMounts, RuntimeCLIVolumeName, RuntimeCLIMountPath, true) || hasMount(sessionAPI.VolumeMounts, RuntimeAgentTokenVolumeName, RuntimeAgentTokenMountPath, true) {
		t.Fatalf("session API must not receive platform CLI or agent token: %#v", sessionAPI)
	}
}

func volumeByName(volumes []corev1.Volume, name string) *corev1.Volume {
	for index := range volumes {
		if volumes[index].Name == name {
			return &volumes[index]
		}
	}
	return nil
}

func projectedAudience(volume *corev1.Volume) string {
	if volume == nil || volume.Projected == nil || len(volume.Projected.Sources) != 1 || volume.Projected.Sources[0].ServiceAccountToken == nil {
		return ""
	}
	return volume.Projected.Sources[0].ServiceAccountToken.Audience
}

func containerByName(containers []corev1.Container, name string) *corev1.Container {
	for index := range containers {
		if containers[index].Name == name {
			return &containers[index]
		}
	}
	return nil
}

func hasMount(mounts []corev1.VolumeMount, name, path string, readOnly bool) bool {
	for _, mount := range mounts {
		if mount.Name == name && mount.MountPath == path && mount.ReadOnly == readOnly {
			return true
		}
	}
	return false
}

func hasEnv(environment []corev1.EnvVar, name, value string) bool {
	for _, item := range environment {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}

func TestDeleteRejectsForeignPVCWhenDataDeletionIsRequested(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: DefaultNamespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, RuntimeIDLabel: "99"}}})}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 1, Name: "nanobot", Namespace: DefaultNamespace, PVCName: "data"}
	if err := manager.Delete(context.Background(), instance, true); err == nil {
		t.Fatal("expected foreign PVC deletion to be rejected")
	}
}

func TestHealthReturnsDeployingBeforePodReady(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	replicas := int32(1)
	if _, err := client.Clientset.AppsV1().Deployments(DefaultNamespace).Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nanobot-main", Namespace: DefaultNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 0},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{Name: "nanobot-main", Namespace: DefaultNamespace, DeploymentMode: model.RuntimeDeploymentManaged, EndpointURL: server.URL, HealthPath: "/health"}
	status, detail := manager.Health(context.Background(), instance)
	if status != model.RuntimeStatusDeploying || !strings.Contains(detail, "尚未就绪") {
		t.Fatalf("expected deploying before readiness, got %s %q", status, detail)
	}
	if hits != 0 {
		t.Fatalf("HTTP probe must not run before readiness, got %d hits", hits)
	}
}

func TestHealthProbesHTTPWhenPodReady(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	replicas := int32(1)
	if _, err := client.Clientset.AppsV1().Deployments(DefaultNamespace).Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nanobot-main", Namespace: DefaultNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{Name: "nanobot-main", Namespace: DefaultNamespace, DeploymentMode: model.RuntimeDeploymentManaged, EndpointURL: server.URL, HealthPath: "/health"}
	status, detail := manager.Health(context.Background(), instance)
	if status != model.RuntimeStatusReady || detail != "Runtime 健康检查通过" {
		t.Fatalf("expected ready after probe, got %s %q", status, detail)
	}
}

func TestHealthSkipsReadinessCheckForExternalRuntime(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset()}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{Name: "nanobot-remote", Namespace: DefaultNamespace, DeploymentMode: model.RuntimeDeploymentExternal, EndpointURL: server.URL, HealthPath: "/health"}
	status, _ := manager.Health(context.Background(), instance)
	if status != model.RuntimeStatusReady {
		t.Fatalf("expected external runtime to probe directly, got %s", status)
	}
}
