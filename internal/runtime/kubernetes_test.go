package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
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

func TestDeleteRejectsForeignPVCWhenDataDeletionIsRequested(t *testing.T) {
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: DefaultNamespace, Labels: map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, RuntimeIDLabel: "99"}}})}
	manager := NewKubernetesManager(client)
	instance := &model.RuntimeInstance{ID: 1, Name: "nanobot", Namespace: DefaultNamespace, PVCName: "data"}
	if err := manager.Delete(context.Background(), instance, true); err == nil {
		t.Fatal("expected foreign PVC deletion to be rejected")
	}
}
