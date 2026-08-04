package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRegistryProxyHandlerDeploysIndependentUpstreamInstances(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})}
	defer func() { K8s = original }()

	handler := NewRegistryProxyHandler(st)
	router := gin.New()
	router.POST("/api/registry-proxies", handler.Deploy)
	router.GET("/api/registry-proxies", handler.List)

	for _, payload := range []string{
		`{"name":"Docker Hub","registry":"docker.io","node_name":"node-a","endpoint_host":"100.64.0.8","node_port":30500,"cache_limit_gi":2,"cleanup_interval_hours":24}`,
		`{"name":"Kubernetes Registry","registry":"registry.k8s.io","node_name":"node-a","endpoint_host":"100.64.0.8","node_port":30501,"cache_limit_gi":2,"cleanup_interval_hours":24}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies", payload))
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected deployment response: %d %s", response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies", `{"name":"Duplicate port","registry":"ghcr.io","node_name":"node-a","endpoint_host":"100.64.0.8","node_port":30501,"cache_limit_gi":2,"cleanup_interval_hours":24}`))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "NodePort 30501") {
		t.Fatalf("expected duplicate NodePort validation error: %d %s", response.Code, response.Body.String())
	}

	proxies, err := st.ListRegistryProxies()
	if err != nil || len(proxies) != 2 {
		t.Fatalf("expected two proxy instances: %#v, %v", proxies, err)
	}
	for _, proxy := range proxies {
		if proxy.ResourceName == "" {
			t.Fatalf("expected resource name for %#v", proxy)
		}
		deployment, getErr := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), proxy.ResourceName, metav1.GetOptions{})
		if getErr != nil {
			t.Fatalf("expected proxy deployment %s: %v", proxy.ResourceName, getErr)
		}
		if proxy.Registry == "registry.k8s.io" && deployment.Spec.Template.Spec.Containers[0].Env[0].Value != "https://registry.k8s.io" {
			t.Fatalf("expected registry.k8s.io upstream, got %#v", deployment.Spec.Template.Spec.Containers[0].Env)
		}
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/registry-proxies", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "registry.k8s.io") {
		t.Fatalf("unexpected list response: %d %s", response.Code, response.Body.String())
	}
}

func TestRegistryProxyHandlerMigratesLegacyDockerHubResources(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &model.RegistryProxy{Name: "Docker Hub 代理", Registry: "docker.io", UpstreamURL: "https://registry-1.docker.io", ResourceName: registryProxyName, NodeName: "node-a", EndpointHost: "100.64.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24, Status: "ready"}
	if err := st.SaveRegistryProxy(proxy); err != nil {
		t.Fatal(err)
	}

	oldDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: registryProxyName, Namespace: registryProxyNamespace}}
	oldService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: registryProxyName, Namespace: registryProxyNamespace}}
	original := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(oldDeployment, oldService)}
	defer func() { K8s = original }()

	handler := NewRegistryProxyHandler(st)
	router := gin.New()
	router.POST("/api/registry-proxies/:id/migrate-resource-name", handler.MigrateResourceName)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/registry-proxies/1/migrate-resource-name", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected migration response: %d %s", response.Code, response.Body.String())
	}

	updated, err := st.GetRegistryProxyByID(proxy.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantResourceName := registryProxyName + "-" + strconv.Itoa(int(proxy.ID))
	if updated.ResourceName != wantResourceName {
		t.Fatalf("expected resource name %q, got %q", wantResourceName, updated.ResourceName)
	}
	if _, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), wantResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected migrated deployment: %v", err)
	}
	if _, err := K8s.Clientset.CoreV1().Services(registryProxyNamespace).Get(context.Background(), wantResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected migrated service: %v", err)
	}
	if _, err := K8s.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), registryProxyName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected legacy deployment to be deleted, got %v", err)
	}
}

func rawJSONRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
