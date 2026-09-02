package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestInstallVictoriaMetricsCreatesSystemManagedPVCDeployment(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}

	status, err := client.InstallVictoriaMetricsContext(context.Background(), VictoriaMetricsConfig{
		NodeName:      "node-a",
		Storage:       "10Gi",
		RetentionDays: 14,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != VictoriaMetricsStateInstalling {
		t.Fatalf("expected installing state, got %#v", status)
	}

	deployment, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected deployment: %v", err)
	}
	if deployment.Spec.Template.Spec.NodeName != "" || deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "node-a" {
		t.Fatalf("expected hostname node selector, got %#v", deployment.Spec.Template.Spec)
	}
	if deployment.Spec.Template.Annotations["cylism.io/scrape-config-hash"] == "" {
		t.Fatalf("expected scrape configuration checksum annotation: %#v", deployment.Spec.Template.Annotations)
	}
	volume := deployment.Spec.Template.Spec.Volumes[0]
	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != victoriaMetricsPVCName {
		t.Fatalf("expected VictoriaMetrics PVC volume, got %#v", volume)
	}
	claim, err := client.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsPVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected VictoriaMetrics PVC: %v", err)
	}
	if claim.Labels[InfrastructureLabel] != InfrastructureVictoriaMetrics || claim.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("expected infrastructure labels, got %#v", claim.Labels)
	}
	if got := claim.Spec.Resources.Requests.Storage().String(); got != "10Gi" {
		t.Fatalf("expected 10Gi PVC request, got %s", got)
	}
	args := strings.Join(deployment.Spec.Template.Spec.Containers[0].Args, " ")
	if !strings.Contains(args, "-retentionPeriod=14d") {
		t.Fatalf("expected retention flag, got %s", args)
	}
	if _, err := client.Clientset.CoreV1().Services(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected service: %v", err)
	}
	scrapeConfig, err := client.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsName+"-scrape", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected scrape config: %v", err)
	}
	if !strings.Contains(scrapeConfig.Data["scrape.yml"], "target_label: node") {
		t.Fatalf("expected scrape config to retain Kubernetes node labels: %s", scrapeConfig.Data["scrape.yml"])
	}
	nodeExporter, err := client.Clientset.AppsV1().DaemonSets(victoriaMetricsNamespace).Get(t.Context(), nodeExporterName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected node-exporter DaemonSet: %v", err)
	}
	if !nodeExporter.Spec.Template.Spec.HostNetwork || nodeExporter.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath != "/host" {
		t.Fatalf("expected node-exporter host metrics configuration: %#v", nodeExporter.Spec.Template.Spec)
	}
	role, err := client.Clientset.RbacV1().ClusterRoles().Get(t.Context(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected VictoriaMetrics ClusterRole: %v", err)
	}
	if len(role.Rules) != 2 || len(role.Rules[1].Verbs) != 1 || role.Rules[1].Verbs[0] != "get" {
		t.Fatalf("expected nodes/proxy to be read-only: %#v", role.Rules)
	}
}

func TestVictoriaMetricsStatusReportsReadyConfiguration(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
				Volumes:      []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: victoriaMetricsPVCName}}}},
				Containers:   []corev1.Container{{Name: "victoria-metrics", Args: []string{"-retentionPeriod=7d"}}},
			}},
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsPVCName, Namespace: victoriaMetricsNamespace, Labels: map[string]string{ManagedByLabel: ManagedByValue, InfrastructureLabel: InfrastructureVictoriaMetrics}}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}}}, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeExporterName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}})}

	status := client.VictoriaMetricsStatus()
	if status.State != VictoriaMetricsStateReady || status.NodeName != "node-a" || status.StorageMode != VictoriaMetricsStoragePVC || status.PVCName != victoriaMetricsPVCName || status.Storage != "10Gi" || status.RetentionDays != 7 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestVictoriaMetricsStatusReportsPartialNodeExporterCoverageAsDegraded(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeExporterName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 2, NumberAvailable: 1}},
	)}

	status := client.VictoriaMetricsStatus()
	if status.State != VictoriaMetricsStateDegraded || status.ReadyReplicas != 1 || status.NodeExporterReady != 1 || status.NodeExporterDesired != 2 {
		t.Fatalf("unexpected partial coverage status: %#v", status)
	}
	if !strings.Contains(status.Message, "1/2") {
		t.Fatalf("expected coverage in status message, got %q", status.Message)
	}
}

func TestInstallVictoriaMetricsRejectsRelocation(t *testing.T) {
	dataPathType := corev1.HostPathDirectoryOrCreate
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
			Volumes:      []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data/old", Type: &dataPathType}}}},
		}}},
	}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		existing,
	)}

	_, err := client.InstallVictoriaMetricsContext(context.Background(), VictoriaMetricsConfig{NodeName: "node-b", Storage: "10Gi", RetentionDays: 14})
	if err == nil || !strings.Contains(err.Error(), "不能直接修改") {
		t.Fatalf("expected relocation error, got %v", err)
	}
}

func TestStartVictoriaMetricsHostPathMigrationCreatesNodeBoundJob(t *testing.T) {
	hostPathType := corev1.HostPathDirectory
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}, Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(1), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
		Volumes:      []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data/victoria-metrics", Type: &hostPathType}}}},
		Containers:   []corev1.Container{{Name: "victoria-metrics", Args: []string{"-retentionPeriod=14d"}}},
	}}}}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		deployment,
	)}

	status, err := client.StartVictoriaMetricsHostPathMigrationContext(context.Background(), VictoriaMetricsMigrationRequest{Storage: "10Gi"})
	if err != nil {
		t.Fatal(err)
	}
	if status.StorageMigration == nil || status.StorageMigration.Stage != VictoriaMetricsMigrationCopying {
		t.Fatalf("expected copying migration state, got %#v", status)
	}
	updated, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil || updated.Spec.Replicas == nil || *updated.Spec.Replicas != 0 {
		t.Fatalf("expected stopped hostPath deployment, got %#v, %v", updated, err)
	}
	job, err := client.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsMigrationJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected migration job: %v", err)
	}
	if job.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "node-a" || !jobUsesHostPathAndPVC(job) {
		t.Fatalf("expected node-bound hostPath/PVC migration job, got %#v", job.Spec.Template.Spec)
	}
}

func TestVictoriaMetricsStatusCompletesSuccessfulHostPathMigration(t *testing.T) {
	hostPathType := corev1.HostPathDirectory
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace}, Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(0), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
		Volumes:      []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data/victoria-metrics", Type: &hostPathType}}}},
		Containers:   []corev1.Container{{Name: "victoria-metrics", Args: []string{"-retentionPeriod=14d"}}},
	}}}}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(
		deployment,
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsPVCName, Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(InfrastructureVictoriaMetrics)}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsMigrationJobName, Namespace: victoriaMetricsNamespace}, Status: batchv1.JobStatus{Succeeded: 1}},
	)}

	status := client.VictoriaMetricsStatus()
	if status.StorageMode != VictoriaMetricsStoragePVC || status.PVCName != victoriaMetricsPVCName {
		t.Fatalf("expected PVC storage after completed migration, got %#v", status)
	}
	updated, err := client.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsName, metav1.GetOptions{})
	if err != nil || updated.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim == nil {
		t.Fatalf("expected cutover deployment, got %#v, %v", updated, err)
	}
	if _, err := client.Clientset.BatchV1().Jobs(victoriaMetricsNamespace).Get(t.Context(), victoriaMetricsMigrationJobName, metav1.GetOptions{}); err == nil {
		t.Fatal("expected completed migration job cleanup")
	}
}

func jobUsesHostPathAndPVC(job *batchv1.Job) bool {
	hasHostPath, hasPVC := false, false
	for _, volume := range job.Spec.Template.Spec.Volumes {
		hasHostPath = hasHostPath || volume.HostPath != nil
		hasPVC = hasPVC || volume.PersistentVolumeClaim != nil
	}
	return hasHostPath && hasPVC
}

func int32Ptr(value int32) *int32 { return &value }
