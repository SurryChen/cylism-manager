package k8s

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestInstallVictoriaMetricsCreatesHostnameSelectedHostPathDeployment(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})}

	status, err := client.InstallVictoriaMetrics(VictoriaMetricsConfig{
		NodeName:      "node-a",
		DataPath:      "/data/victoria-metrics",
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
	if volume.HostPath == nil || volume.HostPath.Path != "/data/victoria-metrics" {
		t.Fatalf("expected hostPath volume, got %#v", volume)
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
	dataPathType := corev1.HostPathDirectoryOrCreate
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: victoriaMetricsName, Namespace: victoriaMetricsNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "node-a"},
				Volumes:      []corev1.Volume{{Name: "storage", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data/victoria-metrics", Type: &dataPathType}}}},
				Containers:   []corev1.Container{{Name: "victoria-metrics", Args: []string{"-retentionPeriod=7d"}}},
			}},
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: nodeExporterName, Namespace: victoriaMetricsNamespace}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, NumberAvailable: 1}})}

	status := client.VictoriaMetricsStatus()
	if status.State != VictoriaMetricsStateReady || status.NodeName != "node-a" || status.DataPath != "/data/victoria-metrics" || status.RetentionDays != 7 {
		t.Fatalf("unexpected status: %#v", status)
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

	_, err := client.InstallVictoriaMetrics(VictoriaMetricsConfig{NodeName: "node-b", DataPath: "/data/new", RetentionDays: 14})
	if err == nil || !strings.Contains(err.Error(), "不能修改") {
		t.Fatalf("expected relocation error, got %v", err)
	}
}

func int32Ptr(value int32) *int32 { return &value }
