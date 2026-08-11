package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func systemComponentScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChart"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartList"}, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfig"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.cattle.io", Version: "v1", Kind: "HelmChartConfigList"}, &unstructured.UnstructuredList{})
	return scheme
}

func setupSystemComponentRouter(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	replicas := int32(1)
	K8s = &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b", Labels: map[string]string{corev1.LabelHostname: "worker-b"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/os": "linux"}}}}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(systemComponentScheme(), &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.cattle.io/v1",
			"kind":       "HelmChart",
			"metadata":   map[string]any{"name": "traefik", "namespace": "kube-system"},
		}}),
	}
	t.Cleanup(func() { K8s = nil })
	_, err = K8s.Clientset.AppsV1().DaemonSets("kube-system").Create(context.Background(), &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "svclb-traefik-abc", Namespace: "kube-system"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create daemonset: %v", err)
	}
	handler := NewSystemComponentHandler(s)
	router := gin.New()
	group := router.Group("/api/system-components")
	group.GET("", handler.List)
	group.PUT("/:chart", handler.Update)
	group.POST("/:chart/revert", handler.Revert)
	return router, s
}

func TestSystemComponentListReturnsWhitelist(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/system-components", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	for _, chart := range []string{"coredns", "traefik", "metrics-server", "local-path-provisioner", "servicelb"} {
		if !strings.Contains(response.Body.String(), chart) {
			t.Fatalf("whitelist chart %s missing: %s", chart, response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), `"lb_active":true`) {
		t.Fatalf("servicelb must be marked active when svclb DaemonSet exists: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"controller_mode":"static_deployment"`) {
		t.Fatalf("coredns control source not reported: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"controller_mode":"helm_chart"`) {
		t.Fatalf("traefik control source not reported: %s", response.Body.String())
	}
}

func TestSystemComponentUpdatePersistsAndApplies(t *testing.T) {
	router, s := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n"}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	config, err := s.GetSystemComponentConfig("coredns")
	if err != nil || config.ApplyStatus != "succeeded" {
		t.Fatalf("config not persisted: %#v err=%v", config, err)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("coredns deployment not updated: %v %#v", err, deployment)
	}
	if deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "worker-b" {
		t.Fatalf("coredns was not pinned to worker-b: %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
	if deployment.Spec.Strategy.RollingUpdate == nil || deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue() != 0 || deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntValue() != 1 {
		t.Fatalf("coredns rollout baseline missing: %#v", deployment.Spec.Strategy)
	}
	obj, err := K8s.GetHelmChartConfig(context.Background(), "kube-system", "coredns")
	if err != nil || obj != nil {
		t.Fatalf("coredns must not use HelmChartConfig: %v %#v", err, obj)
	}
}

func TestSystemComponentUpdateKeepsHelmChartConfigForOtherCharts(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/traefik", gin.H{"values_content": "replicas: 2\n"}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	obj, err := K8s.GetHelmChartConfig(context.Background(), "kube-system", "traefik")
	if err != nil || obj == nil {
		t.Fatalf("traefik helmchartconfig not applied: %v %#v", err, obj)
	}
}

func TestSystemComponentUpdateUsesStaticDeploymentWhenNoHelmChartExists(t *testing.T) {
	router, s := setupSystemComponentRouter(t)
	replicas := int32(1)
	_, err := K8s.Clientset.AppsV1().Deployments("kube-system").Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-server", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create metrics-server deployment: %v", err)
	}
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/metrics-server", gin.H{"values_content": "replicas: 2\nmaxUnavailable: 0\nmaxSurge: 1\n"}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "metrics-server", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("metrics server deployment not updated: %v %#v", err, deployment)
	}
	config, err := s.GetSystemComponentConfig("metrics-server")
	if err != nil || config.ControllerMode != string(k8s.StaticDeploymentMode) {
		t.Fatalf("static config mode not persisted: %#v err=%v", config, err)
	}
}

func TestSystemComponentUpdateRejectsUnknownChart(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/evil", gin.H{"values_content": "replicas: 2\n"}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSystemComponentUpdateRejectsInvalidYAML(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: [unclosed"}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid yaml, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSystemComponentRevertRestoresDefaults(t *testing.T) {
	router, s := setupSystemComponentRouter(t)
	_ = serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n"}))
	response := serve(router, newJSONRequest(http.MethodPost, "/api/system-components/coredns/revert", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("revert status = %d: %s", response.Code, response.Body.String())
	}
	obj, _ := K8s.GetHelmChartConfig(context.Background(), "kube-system", "coredns")
	if obj != nil {
		t.Fatalf("helmchartconfig should be deleted, got %#v", obj)
	}
	if _, err := s.GetSystemComponentConfig("coredns"); err == nil {
		t.Fatal("store config should be removed after revert")
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("coredns deployment defaults not restored: %v %#v", err, deployment)
	}
	if deployment.Spec.Strategy.RollingUpdate == nil || deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue() != 1 || deployment.Spec.Strategy.RollingUpdate.MaxSurge.String() != "25%" {
		t.Fatalf("coredns rollout defaults not restored: %#v", deployment.Spec.Strategy)
	}
	if _, pinned := deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]; pinned {
		t.Fatalf("coredns node pin not removed: %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
}

func TestSystemComponentReconcileStopsWhenControlSourceChanges(t *testing.T) {
	_, s := setupSystemComponentRouter(t)
	if err := s.UpsertSystemComponentConfig(&model.SystemComponentConfig{
		ChartName:      "coredns",
		Namespace:      "kube-system",
		ControllerMode: string(k8s.StaticDeploymentMode),
		ValuesContent:  "replicas: 2\nmaxUnavailable: 0\nmaxSurge: 1\n",
		Enabled:        true,
		ApplyStatus:    "succeeded",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	_, err := K8s.DynamicClient.Resource(schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}).Namespace("kube-system").Create(context.Background(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"metadata":   map[string]any{"name": "coredns", "namespace": "kube-system"},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create HelmChart: %v", err)
	}

	NewSystemComponentHandler(s).reconcileOnce()
	config, err := s.GetSystemComponentConfig("coredns")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ControllerMode != string(k8s.StaticDeploymentMode) || config.ApplyStatus != "failed" || !strings.Contains(config.ApplyError, "已停止自动重放") {
		t.Fatalf("control source change must stop reconcile: %#v", config)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("static deployment must not be patched after source change: %v %#v", err, deployment)
	}
}

func TestSystemComponentEffectiveComparesSavedValuesWithDeployment(t *testing.T) {
	replicas := int32(1)
	unavailable := intstr.FromInt(1)
	surge := intstr.FromString("25%")
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &unavailable,
					MaxSurge:       &surge,
				},
			},
		},
	}
	effective, detail := systemComponentEffective("maxUnavailable: 1\nmaxSurge: 25%\nreplicas: 1\n", deployment)
	if !effective || detail != "" {
		t.Fatalf("matching values must be effective, got %v %q", effective, detail)
	}
	effective, detail = systemComponentEffective("maxUnavailable: 0\nmaxSurge: 1\n", deployment)
	if effective || !strings.Contains(detail, "maxUnavailable") || !strings.Contains(detail, "maxSurge") {
		t.Fatalf("expected both strategy values ineffective, got %v %q", effective, detail)
	}
	effective, detail = systemComponentEffective("replicas: 2\n", deployment)
	if effective || !strings.Contains(detail, "replicas") {
		t.Fatalf("expected replicas ineffective, got %v %q", effective, detail)
	}
	effective, _ = systemComponentEffective("maxUnavailable: 0\n", &appsv1.Deployment{})
	if effective {
		t.Fatal("non-rolling strategy must report ineffective")
	}
	replicas = 2
	deployment.Spec.Template.Spec.NodeSelector = map[string]string{corev1.LabelHostname: "worker-b"}
	effective, detail = systemComponentEffective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n", deployment)
	if effective || !strings.Contains(detail, "maxUnavailable") || !strings.Contains(detail, "maxSurge") {
		t.Fatalf("nested CoreDNS values must detect rollout drift, got %v %q", effective, detail)
	}
	unavailable = intstr.FromInt(0)
	surge = intstr.FromInt(1)
	effective, detail = systemComponentEffective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n", deployment)
	if !effective || detail != "" {
		t.Fatalf("nested CoreDNS values must be effective, got %v %q", effective, detail)
	}
	effective, detail = systemComponentEffective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector: {}\n", deployment)
	if effective || !strings.Contains(detail, "nodeSelector") {
		t.Fatalf("unfixed CoreDNS values must detect a remaining node pin, got %v %q", effective, detail)
	}
}
