package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

func TestCertManagerStatusReady(t *testing.T) {
	client := certManagerTestClient(t, []runtime.Object{
		customResourceDefinition("certificates.cert-manager.io"),
		customResourceDefinition("issuers.cert-manager.io"),
		customResourceDefinition("clusterissuers.cert-manager.io"),
	}, true)

	status := client.CertManagerStatus()
	if status.State != CertManagerStateReady {
		t.Fatalf("expected ready status, got %+v", status)
	}
}

func TestInstallCertManagerCreatesFixedHelmChart(t *testing.T) {
	client := certManagerTestClient(t, []runtime.Object{customResourceDefinition("helmcharts.helm.cattle.io")}, false)

	status, err := client.InstallCertManager("https://charts.jetstack.io", "cert-manager", "v1.16.3")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != CertManagerStateInstalling {
		t.Fatalf("expected installing status, got %+v", status)
	}
	chart, err := client.DynamicClient.Resource(helmChartGVR).Namespace("kube-system").Get(t.Context(), certManagerHelmName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected HelmChart to be created: %v", err)
	}
	if chart.Object["spec"].(map[string]interface{})["chart"] != "cert-manager" {
		t.Fatalf("unexpected HelmChart: %+v", chart.Object)
	}
}

func TestCertManagerStatusReportsFailedHelmChart(t *testing.T) {
	chart := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"metadata":   map[string]interface{}{"name": certManagerHelmName, "namespace": "kube-system"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Failed", "status": "True", "message": "chart download failed"},
		}},
	}}
	client := certManagerTestClient(t, []runtime.Object{customResourceDefinition("helmcharts.helm.cattle.io"), chart}, false)

	status := client.CertManagerStatus()
	if status.State != CertManagerStateDegraded || status.Message != "cert-manager 安装失败: chart download failed" {
		t.Fatalf("expected failed installation status, got %+v", status)
	}
}

func TestInstallDNSProviderCreatesFixedHelmChart(t *testing.T) {
	client := certManagerTestClient(t, []runtime.Object{
		customResourceDefinition("certificates.cert-manager.io"), customResourceDefinition("issuers.cert-manager.io"), customResourceDefinition("clusterissuers.cert-manager.io"), customResourceDefinition("helmcharts.helm.cattle.io"),
	}, true)
	status, err := client.InstallDNSProvider("alidns")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != CertManagerStateInstalling {
		t.Fatalf("unexpected status: %#v", status)
	}
	provider, _ := GetDNSProvider("alidns")
	definition := provider.WebhookChart()
	chart, err := client.DynamicClient.Resource(helmChartGVR).Namespace("kube-system").Get(t.Context(), definition.ReleaseName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := chart.Object["spec"].(map[string]interface{})
	if spec["repo"] != definition.Repository || spec["chart"] != definition.Chart || spec["version"] != definition.Version {
		t.Fatalf("unexpected provider chart: %#v", spec)
	}
}

func certManagerTestClient(t *testing.T, objects []runtime.Object, ready bool) *Client {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{
		crdGVR:                "CustomResourceDefinitionList",
		certGVR:               "CertificateList",
		issuerGVR:             "IssuerList",
		clusterIssuerGVR:      "ClusterIssuerList",
		certificateRequestGVR: "CertificateRequestList",
		orderGVR:              "OrderList",
		challengeGVR:          "ChallengeList",
		helmChartGVR:          "HelmChartList",
	}
	deployments := []runtime.Object{}
	if ready {
		for _, name := range []string{certManagerHelmName, certManagerHelmName + "-webhook", certManagerHelmName + "-cainjector"} {
			deployments = append(deployments, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: certManagerNamespace}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}})
		}
	}
	return &Client{
		Clientset:     k8sfake.NewSimpleClientset(deployments...),
		DynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...),
	}
}

func customResourceDefinition(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": name},
	}}
}
