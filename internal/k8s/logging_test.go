package k8s

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestInstallLoggingCreatesManagedLokiAndAlloy(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}

	status, err := client.InstallLogging(LoggingConfig{NodeName: "node-a", Storage: "10Gi", RetentionDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != LoggingStateInstalling {
		t.Fatalf("expected installing status, got %#v", status)
	}

	statefulSet, err := client.Clientset.AppsV1().StatefulSets(victoriaMetricsNamespace).Get(t.Context(), lokiName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Loki StatefulSet: %v", err)
	}
	if statefulSet.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "node-a" {
		t.Fatalf("expected node binding, got %#v", statefulSet.Spec.Template.Spec.NodeSelector)
	}
	if statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath != "/loki" {
		t.Fatalf("expected Loki data mount, got %#v", statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), lokiPVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Loki PVC: %v", err)
	}
	if claim.Labels[InfrastructureLabel] != InfrastructureLoki || claim.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("expected Loki infrastructure labels, got %#v", claim.Labels)
	}
	if got := claim.Spec.Resources.Requests.Storage().String(); got != "10Gi" {
		t.Fatalf("expected 10Gi PVC, got %s", got)
	}

	alloy, err := client.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(t.Context(), alloyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Alloy DaemonSet: %v", err)
	}
	for _, volume := range alloy.Spec.Template.Spec.Volumes {
		if volume.Name == "pod-logs" || volume.Name == "container-logs" {
			if volume.HostPath == nil {
				t.Fatalf("expected host path for %s", volume.Name)
			}
		}
	}
	for _, mount := range alloy.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == "pod-logs" || mount.Name == "container-logs" {
			if !mount.ReadOnly {
				t.Fatalf("expected read-only %s mount", mount.Name)
			}
		}
	}
	config, err := client.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(t.Context(), loggingConfigName, metav1.GetOptions{})
	if err != nil || !strings.Contains(config.Data["config.alloy"], "stage.cri") || !strings.Contains(config.Data["loki.yaml"], "retention_period: 336h") || !strings.Contains(config.Data["loki.yaml"], "delete_request_store: filesystem") {
		t.Fatalf("expected logging configuration, config=%#v err=%v", config, err)
	}
	role, err := client.Clientset.RbacV1().ClusterRoles().Get(t.Context(), alloyName, metav1.GetOptions{})
	if err != nil || len(role.Rules) != 1 || strings.Join(role.Rules[0].Verbs, ",") != "get,list,watch" {
		t.Fatalf("expected read-only Alloy RBAC, role=%#v err=%v", role, err)
	}
}

func TestLoggingStatusReportsReadyResources(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: lokiName, Namespace: victoriaMetricsNamespace, Annotations: map[string]string{loggingRetentionAnnotation: "7"}},
			Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
				Volumes:      []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: lokiPVCName}}}},
			}}},
			Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
		},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: alloyName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 2, NumberAvailable: 2}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: lokiPVCName, Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(InfrastructureLoki)}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}}},
	)}

	status := client.LoggingStatus()
	if status.State != LoggingStateReady || status.NodeName != "node-a" || status.PVCName != lokiPVCName || status.RetentionDays != 7 || status.AlloyReady != 2 {
		t.Fatalf("unexpected logging status: %#v", status)
	}
}

func TestInstallLoggingRejectsRelocationAndUninstallRetainsPVC(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: lokiName, Namespace: victoriaMetricsNamespace},
			Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
				Volumes:      []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: lokiPVCName}}}},
			}}},
		},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: lokiPVCName, Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(InfrastructureLoki)}},
	)}

	if _, err := client.InstallLogging(LoggingConfig{NodeName: "node-b", RetentionDays: 14}); err == nil || !strings.Contains(err.Error(), "不能直接修改") {
		t.Fatalf("expected relocation error, got %v", err)
	}
	if err := client.UninstallLogging(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), lokiPVCName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected retained Loki PVC: %v", err)
	}
}
