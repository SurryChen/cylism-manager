package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestVictoriaMetricsMigrationJobStatusHelpers(t *testing.T) {
	failed := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "copy failed"}}}}
	if !jobFailed(failed) || jobSucceeded(failed) || jobStatusMessage(failed) != "copy failed" {
		t.Fatalf("unexpected failed job classification")
	}
	succeeded := &batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}
	if !jobSucceeded(succeeded) || jobFailed(succeeded) {
		t.Fatalf("unexpected succeeded job classification")
	}
}

func TestVictoriaMetricsMigrationFailureRestoresReplicas(t *testing.T) {
	replicas := int32(0)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: "node-a"}, Volumes: []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/legacy"}}}}}}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsMigrationJobName, Namespace: victoriaMetricsNamespace}, Status: batchv1.JobStatus{Failed: 1}},
	)}
	status := client.VictoriaMetricsStatus()
	if status.StorageMigration == nil || status.StorageMigration.Stage != VictoriaMetricsMigrationFailed {
		t.Fatalf("expected failed migration status: %#v", status)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(context.Background(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("expected replica recovery, got %#v %v", deployment, err)
	}
}

func TestStartVictoriaMetricsMigrationRejectsMissingInstallation(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	_, err := client.StartVictoriaMetricsHostPathMigrationContext(context.Background(), VictoriaMetricsMigrationRequest{Storage: "10Gi"})
	if err == nil || !strings.Contains(err.Error(), "尚未安装") {
		t.Fatalf("expected missing installation error, got %v", err)
	}
}
