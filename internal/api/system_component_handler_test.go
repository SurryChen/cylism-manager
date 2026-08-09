package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func systemComponentScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
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
	K8s = &k8s.Client{
		Clientset:     k8sfake.NewSimpleClientset(),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(systemComponentScheme()),
	}
	t.Cleanup(func() { K8s = nil })
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
}

func TestSystemComponentUpdatePersistsAndApplies(t *testing.T) {
	router, s := setupSystemComponentRouter(t)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\nmaxUnavailable: 0\n"}))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	config, err := s.GetSystemComponentConfig("coredns")
	if err != nil || config.ApplyStatus != "succeeded" {
		t.Fatalf("config not persisted: %#v err=%v", config, err)
	}
	obj, err := K8s.GetHelmChartConfig(context.Background(), "kube-system", "coredns")
	if err != nil || obj == nil {
		t.Fatalf("helmchartconfig not applied: %v %#v", err, obj)
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
	_ = serve(router, newJSONRequest(http.MethodPut, "/api/system-components/coredns", gin.H{"values_content": "replicas: 2\n"}))
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
}
