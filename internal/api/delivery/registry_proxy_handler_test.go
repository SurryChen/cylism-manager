package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type testRegistryProxyDiagnostics struct {
	diagnose func(context.Context, *model.RegistryProxy) (k8sclient.RegistryProxyDiagnostic, error)
}

func (d testRegistryProxyDiagnostics) Available() bool { return true }
func (d testRegistryProxyDiagnostics) DeploymentAvailable(context.Context, *model.RegistryProxy) (bool, bool, error) {
	return true, false, nil
}
func (d testRegistryProxyDiagnostics) DiagnoseUpstream(ctx context.Context, proxy *model.RegistryProxy) (k8sclient.RegistryProxyDiagnostic, error) {
	return d.diagnose(ctx, proxy)
}

func TestRegistryProxyOutboundProxyIsEncryptedAndDiagnosticIsBounded(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &model.RegistryProxy{Name: "Docker Hub", Registry: "docker.io", UpstreamURL: "https://registry-1.docker.io", ResourceName: "cylism-registry-proxy-1", NodeName: "node-a", EndpointHost: "100.64.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24, Status: "ready"}
	if err := st.SaveRegistryProxy(proxy); err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})}
	reconciler := k8sclient.NewRegistryProxyReconciler(client)
	handler := newTestRegistryProxyHandler(st, []byte("01234567890123456789012345678901"), reconciler, reconciler).WithDiagnostics(testRegistryProxyDiagnostics{diagnose: func(_ context.Context, _ *model.RegistryProxy) (k8sclient.RegistryProxyDiagnostic, error) {
		return k8sclient.RegistryProxyDiagnostic{Status: "upstream_connect_timeout", ResolvedIPs: []string{"128.121.243.75"}, ElapsedMS: 5000, Summary: "upstream timed out"}, nil
	}})
	router := gin.New()
	router.PUT("/api/registry-proxies/:id", handler.Deploy)
	router.POST("/api/registry-proxies/:id/diagnose", handler.Diagnose)
	router.GET("/api/registry-proxies", handler.List)

	payload := `{"name":"Docker Hub","registry":"docker.io","node_name":"node-a","endpoint_host":"100.64.0.8","node_port":30500,"cache_limit_gi":2,"cleanup_interval_hours":24,"http_proxy":"http://user:secret@proxy.internal:3128","no_proxy":".cluster.local"}`
	response := httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPut, "/api/registry-proxies/1", payload))
	if response.Code != http.StatusOK {
		t.Fatalf("update response: %d %s", response.Code, response.Body.String())
	}
	stored, err := st.GetRegistryProxyByID(proxy.ID)
	if err != nil || stored.EncryptedHTTPProxy == "" || strings.Contains(stored.EncryptedHTTPProxy, "secret") {
		t.Fatalf("proxy must be encrypted: %#v err=%v", stored, err)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/registry-proxies/1/diagnose", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "upstream_connect_timeout") {
		t.Fatalf("diagnostic response: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/registry-proxies", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "user:secret") || !strings.Contains(response.Body.String(), `"outbound_proxy_configured":true`) {
		t.Fatalf("unsafe proxy list: %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
}

func TestRegistryProxyListReturnsPersistedStatusSnapshot(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &model.RegistryProxy{Name: "Docker Hub", Registry: "docker.io", ResourceName: "cylism-registry-proxy-1", NodeName: "node-a", EndpointHost: "100.64.0.8", NodePort: 30500, CacheLimitGi: 2, CleanupIntervalHours: 24, Status: "ready"}
	if err := st.SaveRegistryProxy(proxy); err != nil {
		t.Fatal(err)
	}
	handler := newTestRegistryProxyHandler(st, nil, k8sclient.NewRegistryProxyReconciler(&k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}), nil)
	router := gin.New()
	router.GET("/api/registry-proxies", handler.List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/registry-proxies", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ready"`) {
		t.Fatalf("list must return the persisted snapshot: %d %s", response.Code, response.Body.String())
	}
}

func TestImportExistingRegistryProxyOnlyReadsSelectedCluster(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	name := "legacy-registry-proxy"
	cacheLimit := resource.MustParse("10Gi")
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: registryProxyNamespace}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: "node-a"}, Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &cacheLimit}}}}, Containers: []corev1.Container{{Name: "registry", Image: "registry:2.8", Env: []corev1.EnvVar{{Name: "REGISTRY_PROXY_REMOTEURL", Value: "https://registry-1.docker.io"}}}}}}}, Status: appsv1.DeploymentStatus{AvailableReplicas: 1}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: registryProxyNamespace}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"app": name}, Ports: []corev1.ServicePort{{Port: 5000, TargetPort: intstr.FromInt(5000), NodePort: 30500}}}}
	selected := k8sfake.NewSimpleClientset(deployment, service, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	defaultClient := k8sfake.NewSimpleClientset()
	defaultAdapter := k8sclient.NewRegistryProxyReconciler(&k8sclient.Client{Clientset: defaultClient})
	handler := newTestRegistryProxyHandler(st, nil, defaultAdapter, defaultAdapter).WithClusterClientFactory(func(_ context.Context, clusterID uint) (*k8sclient.Client, error) {
		if clusterID != 7 {
			t.Fatalf("unexpected cluster id %d", clusterID)
		}
		return &k8sclient.Client{Clientset: selected}, nil
	})
	router := gin.New()
	router.POST("/api/registry-proxies/import", handler.ImportExisting)
	router.PUT("/api/registry-proxies/:id", handler.Deploy)
	router.POST("/api/registry-proxies/:id/cleanup", handler.Cleanup)
	router.POST("/api/registry-proxies/:id/take-over", handler.TakeOver)
	mismatch := `{"cluster_id":7,"name":"错配代理","resource_name":"legacy-registry-proxy","endpoint_host":"100.76.53.48","registry":"ghcr.io"}`
	response := httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies/import", mismatch))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched registry accepted: %d %s", response.Code, response.Body.String())
	}
	payload := `{"cluster_id":7,"name":"Docker Hub 旧代理","resource_name":"legacy-registry-proxy","endpoint_host":"100.76.53.48","registry":"docker.io"}`
	response = httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies/import", payload))
	if response.Code != http.StatusOK {
		t.Fatalf("import response: %d %s", response.Code, response.Body.String())
	}
	proxies, err := st.ListRegistryProxies()
	if err != nil || len(proxies) != 1 || proxies[0].ClusterID != 7 || proxies[0].ManagementMode != "observe" || proxies[0].NodePort != 30500 {
		t.Fatalf("unexpected imported proxy: %#v, %v", proxies, err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPut, "/api/registry-proxies/"+strconv.Itoa(int(proxies[0].ID)), `{"name":"changed"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("observation allowed configuration: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/registry-proxies/"+strconv.Itoa(int(proxies[0].ID))+"/cleanup", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("observation allowed cache cleanup: %d %s", response.Code, response.Body.String())
	}
	for _, action := range append(selected.Actions(), defaultClient.Actions()...) {
		if action.GetVerb() != "get" {
			t.Fatalf("import changed Kubernetes resources: %s %s", action.GetVerb(), action.GetResource().Resource)
		}
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies/import", payload))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate import accepted: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies/"+strconv.Itoa(int(proxies[0].ID))+"/take-over", `{}`))
	if response.Code != http.StatusOK {
		t.Fatalf("take over response: %d %s", response.Code, response.Body.String())
	}
	stored, err := st.GetRegistryProxyByID(proxies[0].ID)
	if err != nil || stored.ManagementMode != "managed" {
		t.Fatalf("take over state: %#v %v", stored, err)
	}
	for _, action := range selected.Actions() {
		if action.GetVerb() != "get" {
			t.Fatalf("take over changed Kubernetes resources: %s %s", action.GetVerb(), action.GetResource().Resource)
		}
	}
}

func TestImportExistingRegistryProxyRejectsUnrelatedService(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "proxy", Namespace: registryProxyNamespace}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "proxy"}}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: "node-a"}, Containers: []corev1.Container{{Image: "registry:2.8", Env: []corev1.EnvVar{{Name: "REGISTRY_PROXY_REMOTEURL", Value: "https://registry-1.docker.io"}}}}}}}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "proxy", Namespace: registryProxyNamespace}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: map[string]string{"app": "unrelated"}, Ports: []corev1.ServicePort{{NodePort: 30500}}}}
	clientset := k8sfake.NewSimpleClientset(deployment, service)
	handler := newTestRegistryProxyHandler(st, nil, nil, nil).WithClusterClientFactory(func(context.Context, uint) (*k8sclient.Client, error) {
		return &k8sclient.Client{Clientset: clientset}, nil
	})
	router := gin.New()
	router.POST("/api/registry-proxies/import", handler.ImportExisting)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies/import", `{"cluster_id":7,"name":"代理","resource_name":"proxy","endpoint_host":"100.76.53.48"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unrelated resources accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestRemoveObservationDeletesOnlyDatabaseRecord(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &model.RegistryProxy{ClusterID: 7, ManagementMode: "observe", Name: "旧代理", ResourceName: "legacy-proxy", NodeName: "node-a", EndpointHost: "100.76.53.48", NodePort: 30500}
	if err := st.SaveRegistryProxy(proxy); err != nil {
		t.Fatal(err)
	}
	handler := newTestRegistryProxyHandler(st, nil, nil, nil)
	router := gin.New()
	router.DELETE("/api/registry-proxies/:id/observation", handler.RemoveObservation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/registry-proxies/"+strconv.Itoa(int(proxy.ID))+"/observation", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("remove observation: %d %s", response.Code, response.Body.String())
	}
	proxies, err := st.ListRegistryProxies()
	if err != nil || len(proxies) != 0 {
		t.Fatalf("observation still saved: %#v %v", proxies, err)
	}
}

type fakeRegistryProxyReconciler struct{}

func (fakeRegistryProxyReconciler) Available() bool                          { return true }
func (fakeRegistryProxyReconciler) EnsureNode(context.Context, string) error { return nil }
func (fakeRegistryProxyReconciler) Apply(context.Context, *model.RegistryProxy, map[string]string) error {
	return nil
}
func (fakeRegistryProxyReconciler) ClearCache(context.Context, *model.RegistryProxy) error {
	return nil
}
func (fakeRegistryProxyReconciler) DeploymentAvailable(context.Context, *model.RegistryProxy) (bool, bool, error) {
	return true, true, nil
}
func (fakeRegistryProxyReconciler) DeleteLegacyResources(context.Context, string) error { return nil }
func (fakeRegistryProxyReconciler) DiagnoseUpstream(context.Context, *model.RegistryProxy) (k8sclient.RegistryProxyDiagnostic, error) {
	return k8sclient.RegistryProxyDiagnostic{Status: "ok"}, nil
}

func TestRegistryProxyHandlerAcceptsFakeReconciler(t *testing.T) {
	fake := fakeRegistryProxyReconciler{}
	h := (&RegistryProxyHandler{}).WithResourceReconciler(fake).WithDiagnostics(fake)
	if h.resources == nil || h.diagnostics == nil || !h.diagnostics.Available() {
		t.Fatal("fake registry proxy reconciler was not injected")
	}
}

func TestRegistryProxyHandlerDeploysIndependentUpstreamInstances(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})}
	reconciler := k8sclient.NewRegistryProxyReconciler(client)
	handler := newTestRegistryProxyHandler(st, nil, reconciler, reconciler)
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
		deployment, getErr := client.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), proxy.ResourceName, metav1.GetOptions{})
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

func TestRegistryProxyHandlerUsesOnlyConfiguredPodDNS(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})}
	reconciler := k8sclient.NewRegistryProxyReconciler(client)
	handler := newTestRegistryProxyHandler(st, nil, reconciler, reconciler)
	router := gin.New()
	router.POST("/api/registry-proxies", handler.Deploy)

	payload := `{"name":"Docker Hub","registry":"docker.io","node_name":"node-a","endpoint_host":"100.64.0.8","node_port":30500,"cache_limit_gi":2,"cleanup_interval_hours":24,"dns_servers":["10.0.0.2","10.0.0.3"]}`
	response := httptest.NewRecorder()
	router.ServeHTTP(response, rawJSONRequest(http.MethodPost, "/api/registry-proxies", payload))
	if response.Code != http.StatusOK {
		t.Fatalf("deploy response: %d %s", response.Code, response.Body.String())
	}
	proxy, err := st.GetRegistryProxy()
	if err != nil || proxy.DNSResolvers != `["10.0.0.2","10.0.0.3"]` {
		t.Fatalf("stored DNS: %#v err=%v", proxy, err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), proxy.ResourceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	podSpec := deployment.Spec.Template.Spec
	if podSpec.DNSPolicy != corev1.DNSNone || podSpec.DNSConfig == nil || strings.Join(podSpec.DNSConfig.Nameservers, ",") != "10.0.0.2,10.0.0.3" {
		t.Fatalf("expected independent DNS config: %#v", podSpec)
	}
}

func TestNormalizeProxyDNSServersRejectsHostLoopback(t *testing.T) {
	if _, err := registryservice.NormalizeProxyDNSServers([]string{"127.0.0.53"}); err == nil {
		t.Fatal("expected loopback DNS to be rejected")
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
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(oldDeployment, oldService)}
	reconciler := k8sclient.NewRegistryProxyReconciler(client)
	handler := newTestRegistryProxyHandler(st, nil, reconciler, reconciler)
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
	if _, err := client.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), wantResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected migrated deployment: %v", err)
	}
	if _, err := client.Clientset.CoreV1().Services(registryProxyNamespace).Get(context.Background(), wantResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected migrated service: %v", err)
	}
	if _, err := client.Clientset.AppsV1().Deployments(registryProxyNamespace).Get(context.Background(), registryProxyName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected legacy deployment to be deleted, got %v", err)
	}
}

func rawJSONRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
