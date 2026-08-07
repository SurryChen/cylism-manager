package k8s

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func opsAgentReadyNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
}

func TestOpsAgentConfigUsesResponsesModelName(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset()}
	if err := client.upsertOpsAgentConfig(OpsAgentConfig{Model: "gpt-4o-mini", BaseURL: "https://responses.example.test/v1"}); err != nil {
		t.Fatal(err)
	}

	configMap, err := client.Clientset.CoreV1().ConfigMaps(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := configMap.Data["OPS_AGENT_MODEL"]; got != "gpt-4o-mini" {
		t.Fatalf("OPS_AGENT_MODEL = %q, want bare Responses API model name", got)
	}
	if got := configMap.Data["OPENAI_BASE_URL"]; got != "https://responses.example.test/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q, want configured Responses API endpoint", got)
	}
}

func TestInstallOpsAgentCreatesDedicatedNamespace(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}
	if _, err := client.InstallOpsAgent(OpsAgentConfig{NodeName: "worker-a", Storage: "1Gi", Model: "gpt-4.1-mini"}, "test-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().Namespaces().Get(client.Ctx(), opsAgentNamespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("dedicated assistant namespace was not created: %v", err)
	}
	if _, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{}); err != nil {
		t.Fatalf("runtime deployment missing from dedicated namespace: %v", err)
	}
	if _, err := client.Clientset.AppsV1().Deployments(legacyOpsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{}); err == nil {
		t.Fatal("new Runtime deployment must not be created in default")
	}
}

func TestCleanupLegacyOpsAgentDeletesOnlyLegacyResources(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: legacyOpsAgentNamespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: legacyOpsAgentNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: legacyOpsAgentNamespace}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-model", Namespace: legacyOpsAgentNamespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-audit", Namespace: legacyOpsAgentNamespace}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-audit", Namespace: opsAgentNamespace}},
	)}
	if err := client.CleanupLegacyOpsAgent(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(legacyOpsAgentNamespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{}); err == nil {
		t.Fatal("legacy audit PVC was not deleted")
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{}); err != nil {
		t.Fatalf("active audit PVC must be retained: %v", err)
	}
}

func TestUninstallOpsAgentRemovesCurrentAndLegacyWorkloadsAndPVCs(t *testing.T) {
	resources := make([]runtime.Object, 0, 10)
	for _, namespace := range []string{opsAgentNamespace, legacyOpsAgentNamespace} {
		resources = append(resources,
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: namespace}},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: namespace}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: namespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-model", Namespace: namespace}},
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-audit", Namespace: namespace}},
		)
	}
	client := &Client{Clientset: fake.NewSimpleClientset(resources...)}

	if err := client.UninstallOpsAgent(); err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{opsAgentNamespace, legacyOpsAgentNamespace} {
		if _, err := client.Clientset.AppsV1().Deployments(namespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("deployment in %s was not removed: %v", namespace, err)
		}
		if _, err := client.Clientset.CoreV1().Services(namespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("service in %s was not removed: %v", namespace, err)
		}
		if _, err := client.Clientset.CoreV1().ConfigMaps(namespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("config map in %s was not removed: %v", namespace, err)
		}
		if _, err := client.Clientset.CoreV1().Secrets(namespace).Get(client.Ctx(), opsAgentName+"-model", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("secret in %s was not removed: %v", namespace, err)
		}
		if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("PVC in %s was not removed: %v", namespace, err)
		}
	}
}

func TestPrepareOpsAgentMigrationCreatesTargetPVCBindingPod(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}
	if err := client.PrepareOpsAgentMigration(OpsAgentConfig{NodeName: "worker-a", Storage: "1Gi"}, "42"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{}); err != nil {
		t.Fatalf("target audit PVC missing: %v", err)
	}
	pod, err := client.Clientset.CoreV1().Pods(opsAgentNamespace).Get(client.Ctx(), "cylism-pvc-migrate-ops-agent-42", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("target binding Pod missing: %v", err)
	}
	if pod.Spec.NodeSelector[corev1.LabelHostname] != "worker-a" || pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != opsAgentName+"-audit" {
		t.Fatalf("unexpected binding pod: %#v", pod.Spec)
	}
}

func TestOpsAgentStatusWaitsForTheUpdatedDeploymentTemplate(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: opsAgentNamespace, Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
	})}

	status := client.OpsAgentStatus()
	if status.State != "installing" {
		t.Fatalf("state = %q, want installing while the new template is unobserved", status.State)
	}
	if status.DesiredReplicas != 1 || status.UpdatedReplicas != 1 || status.ReadyReplicas != 1 {
		t.Fatalf("unexpected replica state: %#v", status)
	}

	deployment, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deployment.Status.ObservedGeneration = 2
	if _, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).UpdateStatus(client.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status = client.OpsAgentStatus(); status.State != "ready" {
		t.Fatalf("state = %q, want ready after the updated template is available", status.State)
	}
}

func TestOpsAgentPVCMigratesToInfrastructureLabels(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: opsAgentName + "-audit", Namespace: opsAgentNamespace, Labels: opsAgentLabels()},
	})}
	if err := client.upsertOpsAgentPVC(OpsAgentConfig{Storage: "1Gi"}); err != nil {
		t.Fatal(err)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(opsAgentNamespace).Get(client.Ctx(), opsAgentName+"-audit", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Labels[InfrastructureLabel] != InfrastructureOpsAgent || claim.Labels["cylism.io/component"] != "assistant" {
		t.Fatalf("assistant audit PVC was not migrated to infrastructure labels: %#v", claim.Labels)
	}
}

func TestInstallOpsAgentRetainsDeploymentActiveAuditClaim(t *testing.T) {
	activeClaim := "cylism-ops-agent-audit-migrate-42"
	client := &Client{Clientset: fake.NewSimpleClientset(
		opsAgentReadyNode("worker-a"),
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: activeClaim, Namespace: opsAgentNamespace}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: opsAgentNamespace}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: "worker-a"},
			Volumes:      []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: activeClaim}}}},
		}}}},
	)}

	if _, err := client.InstallOpsAgent(OpsAgentConfig{NodeName: "worker-a", Storage: "1Gi", Model: "gpt-4.1-mini"}, "test-key"); err != nil {
		t.Fatal(err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName; got != activeClaim {
		t.Fatalf("active audit claim = %q, want %q", got, activeClaim)
	}
}

func TestInstallOpsAgentRejectsNodeChangeForBoundLocalAuditClaim(t *testing.T) {
	claimName := opsAgentName + "-audit"
	client := &Client{Clientset: fake.NewSimpleClientset(
		opsAgentReadyNode("worker-a"),
		opsAgentReadyNode("worker-b"),
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: opsAgentNamespace}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "audit-pv", Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "audit-pv"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/rancher/k3s/storage/audit"}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"worker-a"}}}}}}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: opsAgentNamespace}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: "worker-a"},
			Volumes:      []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName}}}},
		}}}},
	)}

	_, err := client.InstallOpsAgent(OpsAgentConfig{NodeName: "worker-b", Storage: "1Gi", Model: "gpt-4.1-mini"}, "test-key")
	if err == nil || !strings.Contains(err.Error(), "迁移 Runtime 存储") {
		t.Fatalf("InstallOpsAgent error = %v, want Runtime migration guidance", err)
	}
}

func TestSwitchOpsAgentAuditPVCResumesAfterDeploymentCutover(t *testing.T) {
	replicas := int32(0)
	targetClaim := "cylism-ops-agent-audit-migrate-42"
	client := &Client{Clientset: fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: targetClaim, Namespace: opsAgentNamespace}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: opsAgentName, Namespace: opsAgentNamespace, Labels: opsAgentLabels()},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "worker-b"},
				Volumes:      []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: targetClaim}}}},
			}}},
		},
	)}
	if err := client.SwitchOpsAgentAuditPVC("cylism-ops-agent-audit", targetClaim, "worker-b", 1); err != nil {
		t.Fatal(err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if *deployment.Spec.Replicas != 1 || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != targetClaim {
		t.Fatalf("resumed cutover changed the active claim or replica count: %#v", deployment.Spec)
	}
}

func TestInstallOpsAgentReusesActiveMigratedAuditClaimAfterUninstall(t *testing.T) {
	activeClaim := "cylism-ops-agent-audit-migrate-42"
	client := &Client{Clientset: fake.NewSimpleClientset(
		opsAgentReadyNode("worker-b"),
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: activeClaim, Namespace: opsAgentNamespace, Labels: map[string]string{ManagedByLabel: ManagedByValue, "cylism.io/component": "assistant", "cylism.io/assistant-active": "true"}}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}},
	)}
	if _, err := client.InstallOpsAgent(OpsAgentConfig{NodeName: "worker-b", Storage: "1Gi", Model: "gpt-4.1-mini"}, "test-key"); err != nil {
		t.Fatal(err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName; got != activeClaim {
		t.Fatalf("reinstalled Runtime claim = %q, want retained migrated claim %q", got, activeClaim)
	}
}
