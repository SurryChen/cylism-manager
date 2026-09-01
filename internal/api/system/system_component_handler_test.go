package system

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	systemcomponentservice "github.com/cylism/cylism-manager/internal/service/system_component"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	k8sClient = &k8s.Client{
		Clientset: k8sfake.NewSimpleClientset(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Labels: map[string]string{corev1.LabelHostname: "worker-a"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")}}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b", Labels: map[string]string{corev1.LabelHostname: "worker-b"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")}}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/os": "linux"}}}}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "traefik", Namespace: "kube-system", Labels: map[string]string{"app.kubernetes.io/name": "traefik"}}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "traefik"}}}}}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "coredns-a", Namespace: "kube-system", Labels: map[string]string{"k8s-app": "coredns"}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "coredns", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}},
		),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(systemComponentScheme(), &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "helm.cattle.io/v1",
			"kind":       "HelmChart",
			"metadata":   map[string]any{"name": "traefik", "namespace": "kube-system"},
		}}),
	}
	t.Cleanup(func() { k8sClient = nil })
	_, err = k8sClient.Clientset.AppsV1().DaemonSets("kube-system").Create(context.Background(), &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "svclb-traefik-abc", Namespace: "kube-system"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create daemonset: %v", err)
	}
	handler := NewSystemComponentHandler(s, k8s.SystemComponentKubernetesAdapter{Client: k8sClient})
	router := gin.New()
	group := router.Group("/api/system-components")
	group.GET("", handler.List)
	group.PUT("/:chart", handler.Update)
	group.POST("/:chart/revert", handler.Revert)
	return router, s
}

func TestSystemComponentUpdateBlocksCoreDNSHAWhenPreflightFails(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	if err := k8sClient.Clientset.CoreV1().Nodes().Delete(context.Background(), "worker-a", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("remove candidate node: %v", err)
	}
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\n"}))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "高可用需要至少 2 个") {
		t.Fatalf("expected HA preflight rejection, got %d: %s", response.Code, response.Body.String())
	}
	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("preflight must not mutate deployment: %v %#v", err, deployment)
	}
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
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\n"}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	config, err := s.GetSystemComponentConfig("coredns")
	if err != nil || config.ApplyStatus != "succeeded" {
		t.Fatalf("config not persisted: %#v err=%v", config, err)
	}
	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("coredns deployment not updated: %v %#v", err, deployment)
	}
	if _, pinned := deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]; pinned {
		t.Fatalf("CoreDNS HA must remain scheduler managed: %#v", deployment.Spec.Template.Spec.NodeSelector)
	}
	if deployment.Spec.Strategy.RollingUpdate == nil || deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue() != 0 || deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntValue() != 1 {
		t.Fatalf("coredns rollout baseline missing: %#v", deployment.Spec.Strategy)
	}
	obj, err := k8sClient.GetHelmChartConfig(context.Background(), "kube-system", "coredns")
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
	obj, err := k8sClient.GetHelmChartConfig(context.Background(), "kube-system", "traefik")
	if err != nil || obj == nil {
		t.Fatalf("traefik helmchartconfig not applied: %v %#v", err, obj)
	}
}

func TestSystemComponentUpdateManagesTraefikReadTimeout(t *testing.T) {
	router, s := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/traefik", gin.H{
		"values_content":       "maxUnavailable: 0\nmaxSurge: 1\n",
		"traefik_read_timeout": "30m",
	}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	config, err := s.GetSystemComponentConfig("traefik")
	if err != nil || !strings.Contains(config.ValuesContent, "--entryPoints.web.transport.respondingTimeouts.readTimeout=30m") || !strings.Contains(config.ValuesContent, "--entryPoints.websecure.transport.respondingTimeouts.readTimeout=30m") {
		t.Fatalf("stored Traefik values missing entrypoint arguments: %#v err=%v", config, err)
	}
	obj, err := k8sClient.GetHelmChartConfig(context.Background(), "kube-system", "traefik")
	if err != nil || obj == nil {
		t.Fatalf("traefik helmchartconfig not applied: %v %#v", err, obj)
	}
	values, _, _ := unstructured.NestedString(obj.Object, "spec", "valuesContent")
	if !strings.Contains(values, "readTimeout=30m") || strings.Count(values, "readTimeout=30m") != 2 {
		t.Fatalf("values must render two readTimeout args: %q", values)
	}

	response = serve(router, newJSONRequest(http.MethodGet, "/api/system-components", nil))
	if !strings.Contains(response.Body.String(), `"read_timeout_effective":false`) {
		t.Fatalf("timeout must be pending before Helm rolls Traefik: %s", response.Body.String())
	}

	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "traefik", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get traefik deployment: %v", err)
	}
	deployment.Spec.Template.Spec.Containers[0].Args = []string{
		"--entryPoints.web.transport.respondingTimeouts.readTimeout=30m",
		"--entryPoints.websecure.transport.respondingTimeouts.readTimeout=30m",
	}
	if _, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Update(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update traefik deployment: %v", err)
	}
	response = serve(router, newJSONRequest(http.MethodGet, "/api/system-components", nil))
	if !strings.Contains(response.Body.String(), `"read_timeout_effective":true`) || !strings.Contains(response.Body.String(), `"effective_read_timeout":"30m"`) {
		t.Fatalf("timeout must become effective after rollout: %s", response.Body.String())
	}
}

func TestSystemComponentUpdateRejectsInvalidTraefikReadTimeout(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	for _, testCase := range []struct {
		name    string
		chart   string
		timeout string
	}{
		{name: "zero", chart: "traefik", timeout: "0s"},
		{name: "too short", chart: "traefik", timeout: "30s"},
		{name: "too long", chart: "traefik", timeout: "2h"},
		{name: "malformed", chart: "traefik", timeout: "forever"},
		{name: "other chart", chart: "coredns", timeout: "30m"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/"+testCase.chart, gin.H{
				"values_content":       "replicas: 1\nmaxUnavailable: 0\nmaxSurge: 1\n",
				"traefik_read_timeout": testCase.timeout,
			}))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected validation error, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSystemComponentUpdateRejectsUnsupportedStaticReplicaIncrease(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	replicas := int32(1)
	_, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-server", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create metrics-server deployment: %v", err)
	}
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/metrics-server", gin.H{"values_content": "replicas: 2\nmaxUnavailable: 0\nmaxSurge: 1\n"}))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "副本由 K3s/组件 profile 管理") {
		t.Fatalf("expected profile rejection, got %d: %s", response.Code, response.Body.String())
	}
	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "metrics-server", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("metrics server replica count must remain unchanged: %v %#v", err, deployment)
	}
}

func TestSystemComponentListReportsComponentSpecificAvailability(t *testing.T) {
	router, _ := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/system-components", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"high_availability":true`) || !strings.Contains(response.Body.String(), `"safe_baseline":true`) {
		t.Fatalf("coredns availability profile missing: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"local-path-provisioner"`) || !strings.Contains(response.Body.String(), `"high_availability":false`) {
		t.Fatalf("singleton availability profile missing: %s", response.Body.String())
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
	_ = serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 1\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n"}))
	response := serve(router, newJSONRequest(http.MethodPost, "/api/system-components/coredns/revert", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("revert status = %d: %s", response.Code, response.Body.String())
	}
	obj, _ := k8sClient.GetHelmChartConfig(context.Background(), "kube-system", "coredns")
	if obj != nil {
		t.Fatalf("helmchartconfig should be deleted, got %#v", obj)
	}
	if _, err := s.GetSystemComponentConfig("coredns"); err == nil {
		t.Fatal("store config should be removed after revert")
	}
	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
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
	_, err := k8sClient.DynamicClient.Resource(schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}).Namespace("kube-system").Create(context.Background(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"metadata":   map[string]any{"name": "coredns", "namespace": "kube-system"},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create HelmChart: %v", err)
	}

	adapter := k8s.SystemComponentKubernetesAdapter{Client: k8sClient}
	if err := systemcomponentservice.Reconcile(context.Background(), s, adapter, systemcomponentservice.ParseStaticDeploymentConfig, func(ctx context.Context, node string) error {
		return systemcomponentservice.ValidateNode(ctx, adapter, node)
	}, time.Now()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	config, err := s.GetSystemComponentConfig("coredns")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ControllerMode != string(k8s.StaticDeploymentMode) || config.ApplyStatus != "failed" || !strings.Contains(config.ApplyError, "已停止自动重放") {
		t.Fatalf("control source change must stop reconcile: %#v", config)
	}
	deployment, err := k8sClient.Clientset.AppsV1().Deployments("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
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
	effective, detail := systemcomponentservice.Effective("maxUnavailable: 1\nmaxSurge: 25%\nreplicas: 1\n", deployment)
	if !effective || detail != "" {
		t.Fatalf("matching values must be effective, got %v %q", effective, detail)
	}
	effective, detail = systemcomponentservice.Effective("maxUnavailable: 0\nmaxSurge: 1\n", deployment)
	if effective || !strings.Contains(detail, "maxUnavailable") || !strings.Contains(detail, "maxSurge") {
		t.Fatalf("expected both strategy values ineffective, got %v %q", effective, detail)
	}
	effective, detail = systemcomponentservice.Effective("replicas: 2\n", deployment)
	if effective || !strings.Contains(detail, "replicas") {
		t.Fatalf("expected replicas ineffective, got %v %q", effective, detail)
	}
	effective, _ = systemcomponentservice.Effective("maxUnavailable: 0\n", &appsv1.Deployment{})
	if effective {
		t.Fatal("non-rolling strategy must report ineffective")
	}
	replicas = 2
	deployment.Spec.Template.Spec.NodeSelector = map[string]string{corev1.LabelHostname: "worker-b"}
	effective, detail = systemcomponentservice.Effective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n", deployment)
	if effective || !strings.Contains(detail, "maxUnavailable") || !strings.Contains(detail, "maxSurge") {
		t.Fatalf("nested CoreDNS values must detect rollout drift, got %v %q", effective, detail)
	}
	unavailable = intstr.FromInt(0)
	surge = intstr.FromInt(1)
	effective, detail = systemcomponentservice.Effective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector:\n  kubernetes.io/hostname: worker-b\n", deployment)
	if !effective || detail != "" {
		t.Fatalf("nested CoreDNS values must be effective, got %v %q", effective, detail)
	}
	effective, detail = systemcomponentservice.Effective("replicas: 2\ndeploymentStrategy:\n  type: RollingUpdate\n  rollingUpdate:\n    maxUnavailable: 0\n    maxSurge: 1\nnodeSelector: {}\n", deployment)
	if effective || !strings.Contains(detail, "nodeSelector") {
		t.Fatalf("unfixed CoreDNS values must detect a remaining node pin, got %v %q", effective, detail)
	}
}

type countingSystemComponentAdapter struct {
	fakeSystemComponentAdapter
	applyHelm  int
	deleteHelm int
}

func (f *countingSystemComponentAdapter) HasDynamicClient() bool { return true }
func (f *countingSystemComponentAdapter) DetectSystemComponent(context.Context, string, string) (k8s.SystemComponentDetection, error) {
	return k8s.SystemComponentDetection{Mode: k8s.HelmChartMode}, nil
}
func (f *countingSystemComponentAdapter) ApplyHelmChartConfig(context.Context, string, string, string) error {
	f.applyHelm++
	return nil
}
func (f *countingSystemComponentAdapter) DeleteHelmChartConfig(context.Context, string, string) error {
	f.deleteHelm++
	return nil
}

func TestSystemComponentHandlerDelegatesUpdateAndRevertToServiceAdapter(t *testing.T) {
	adapter := &countingSystemComponentAdapter{fakeSystemComponentAdapter: fakeSystemComponentAdapter{client: k8sfake.NewSimpleClientset()}}
	handler := (&SystemComponentHandler{}).WithAdapter(adapter)
	router := gin.New()
	router.PUT("/api/system-components/:chart", handler.Update)
	router.POST("/api/system-components/:chart/revert", handler.Revert)
	values := gin.H{"values_content": "replicas: 1\nmaxUnavailable: 0\nmaxSurge: 1\n"}
	if response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/traefik", values)); response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	if response := serve(router, newJSONRequest(http.MethodPost, "/api/system-components/traefik/revert", nil)); response.Code != http.StatusOK {
		t.Fatalf("revert status = %d: %s", response.Code, response.Body.String())
	}
	if adapter.applyHelm != 1 || adapter.deleteHelm != 1 {
		t.Fatalf("expected service delegation, apply=%d delete=%d", adapter.applyHelm, adapter.deleteHelm)
	}
}

func TestSystemComponentHandlerRunDelegatesReconciliationLifecycle(t *testing.T) {
	store, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &countingSystemComponentAdapter{fakeSystemComponentAdapter: fakeSystemComponentAdapter{client: k8sfake.NewSimpleClientset()}}
	handler := (&SystemComponentHandler{}).WithAdapter(adapter)
	handler.configs = store
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := handler.Run(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("expected canceled lifecycle, got %v", err)
	}
}
