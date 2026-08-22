package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupApplicationRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	projects := r.Group("/api/projects")
	{
		projects.GET("", h.ListProjects)
		projects.POST("", h.CreateProject)
		projects.PUT("/:projectID", h.UpdateProject)
		projects.DELETE("/:projectID", h.DeleteProject)
		projects.GET("/:projectID/environments", h.ListEnvironments)
		projects.GET("/environments/namespace-conflicts", h.ListEnvironmentNamespaceConflicts)
		projects.POST("/:projectID/environments", h.CreateEnvironment)
		projects.PUT("/:projectID/environments/:environmentID", h.UpdateEnvironment)
		projects.POST("/:projectID/environments/:environmentID/sync-namespace", h.SyncEnvironmentNamespace)
		projects.DELETE("/:projectID/environments/:environmentID", h.DeleteEnvironment)
	}
	applications := r.Group("/api/applications")
	{
		applications.GET("", h.ListApplications)
		applications.GET("/discovery", h.DiscoverApplications)
		applications.POST("", h.CreateApplication)
		applications.PUT("/:id/capabilities", h.UpdateCapabilities)
		applications.PUT("/:id/workload-kind", h.UpdateWorkloadKind)
		applications.GET("/:id/runtime", h.GetApplicationRuntime)
		applications.GET("/:id/deployment-templates", h.ListDeploymentTemplates)
		applications.POST("/:id/deployment-templates", h.CreateDeploymentTemplate)
		applications.GET("/:id/deployment-templates/:templateID", h.GetDeploymentTemplate)
		applications.PUT("/:id/deployment-templates/:templateID", h.UpdateDeploymentTemplate)
		applications.DELETE("/:id/deployment-templates/:templateID", h.DeleteDeploymentTemplate)
		applications.POST("/:id/deployment-templates/:templateID/default", h.SetDefaultDeploymentTemplate)
		applications.GET("/:id/endpoints", h.ListApplicationEndpoints)
		applications.POST("/:id/endpoints", h.CreateApplicationEndpoint)
		applications.PUT("/:id/endpoints/:endpointID", h.UpdateApplicationEndpoint)
		applications.DELETE("/:id/endpoints/:endpointID", h.DeleteApplicationEndpoint)
		applications.POST("/:id/releases", h.CreateRelease)
		applications.GET("/:id/releases/:releaseID", h.GetRelease)
		applications.POST("/:id/integration-handoffs", h.CreateIntegrationHandoff)
	}
	workspace := r.Group("/api/workspace")
	workspace.GET("/overview", h.WorkspaceOverview)
	return r, s
}

func TestIntegrationHandoffOnlyAllowsProtectedConsoleAndLoopbackDebugURL(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	protected := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "console.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessProtectedConsole}
	if err := s.CreateApplicationEndpoint(protected); err != nil {
		t.Fatalf("create protected endpoint: %v", err)
	}
	public := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "api.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessPublic}
	if err := s.CreateApplicationEndpoint(public); err != nil {
		t.Fatalf("create public endpoint: %v", err)
	}

	local := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "http://localhost:5178/console?source=test"}))
	if local.Code != http.StatusOK || !strings.Contains(local.Body.String(), "http://localhost:5178/console?handoff_code=") || !strings.Contains(local.Body.String(), "source=test") {
		t.Fatalf("unexpected local handoff response: %d %s", local.Code, local.Body.String())
	}

	nonLoopback := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "https://example.com"}))
	if nonLoopback.Code != http.StatusBadRequest || !strings.Contains(nonLoopback.Body.String(), "本地联调地址无效") {
		t.Fatalf("unexpected non-loopback response: %d %s", nonLoopback.Code, nonLoopback.Body.String())
	}

	publicResult := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": public.ID}))
	if publicResult.Code != http.StatusBadRequest || !strings.Contains(publicResult.Body.String(), "不是受保护控制台") {
		t.Fatalf("unexpected public endpoint response: %d %s", publicResult.Code, publicResult.Body.String())
	}
}

func TestApplicationHandlerUpdatesNormalizedCapabilities(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)

	response := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/capabilities", gin.H{"capabilities": []string{"metrics", " hysteria2 ", "metrics"}}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"capabilities":["hysteria2","metrics"]`) {
		t.Fatalf("unexpected capability update response: %d %s", response.Code, response.Body.String())
	}
	stored, err := s.GetApplication(app.ID)
	if err != nil || len(stored.Capabilities) != 2 || stored.Capabilities[0] != "hysteria2" || stored.Capabilities[1] != "metrics" {
		t.Fatalf("unexpected stored capabilities: %#v err=%v", stored.Capabilities, err)
	}

	invalid := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/capabilities", gin.H{"capabilities": []string{"not valid"}}))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid capability status = %d: %s", invalid.Code, invalid.Body.String())
	}
	stored, err = s.GetApplication(app.ID)
	if err != nil || len(stored.Capabilities) != 2 {
		t.Fatalf("invalid update must preserve stored capabilities: %#v err=%v", stored.Capabilities, err)
	}
}

func TestApplicationHandlerDiscoversCapabilityAndSanitizedRuntime(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	if _, err := s.ReplaceApplicationCapabilities(app.ID, []string{"hysteria2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 4, Version: "2.12.1", Image: "ghcr.io/example/hysteria:2.12.1", DesiredSpec: `{"secrets":{"api":"private"}}`, Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	replicas := int32(2)
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Environment.Namespace},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{{Name: "proxy", Port: 8443, TargetPort: intstr.FromInt32(8443), Protocol: corev1.ProtocolUDP}, {Name: "traffic-api", Port: 10001, TargetPort: intstr.FromInt32(10001), Protocol: corev1.ProtocolTCP}}},
			Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.20"}}}},
		},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Environment.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "hysteria-ready", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name}}, Spec: corev1.PodSpec{NodeName: "worker-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "hysteria", Ready: true}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "hysteria-pending", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name}}, Spec: corev1.PodSpec{NodeName: "worker-b"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
	)}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodGet, "/api/applications/discovery?project_id=1&environment_id=1&capability=hysteria2", nil))
	body := response.Body.String()
	for _, expected := range []string{`"protocol":"UDP"`, `"load_balancer_addresses":["203.0.113.20"]`, `"ready_replicas":1`, `"desired_replicas":2`, `"node_name":"worker-a"`, `"version":"2.12.1"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("discovery response missing %s: %s", expected, body)
		}
	}
	if response.Code != http.StatusOK || strings.Contains(body, "private") || strings.Contains(body, "tls_secret_name") || strings.Contains(body, "desired_spec") {
		t.Fatalf("unexpected discovery response: %d %s", response.Code, body)
	}

	runtime := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/runtime", nil))
	if runtime.Code != http.StatusOK || !strings.Contains(runtime.Body.String(), `"service_name":"browser"`) || !strings.Contains(runtime.Body.String(), `"node_name":"worker-a"`) {
		t.Fatalf("unexpected application runtime response: %d %s", runtime.Code, runtime.Body.String())
	}
}

func TestApplicationHandlerDiscoveryRejectsEnvironmentOutsideProject(t *testing.T) {
	r, s := setupApplicationRouter()
	createApplicationForReleaseRuntimeTest(t, s)
	otherProject := &model.Project{Name: "other-project", OwnerID: 1}
	if err := s.CreateProject(otherProject); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: otherProject.ID, Name: "dev", Namespace: "other-dev"}); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodGet, "/api/applications/discovery?project_id=1&environment_id=2", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected cross-project discovery response: %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationHandlerSetsWorkloadKindBeforeFirstRelease(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/workload-kind", gin.H{"workload_kind": "statefulset"}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "statefulset") {
		t.Fatalf("unexpected workload kind response: %d %s", response.Code, response.Body.String())
	}
	stored, err := s.GetApplication(app.ID)
	if err != nil || stored.WorkloadKind != application.WorkloadKindStatefulSet {
		t.Fatalf("expected StatefulSet application kind, got %+v err=%v", stored, err)
	}
}

func TestWorkspaceOverviewIncludesApplicationRuntimeSummary(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	active := &model.Release{ApplicationID: app.ID, Sequence: 2, Version: "1.4.0", Image: "registry.example.com/browser:1.4.0", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
	failed := &model.Release{ApplicationID: app.ID, Sequence: 3, Version: "1.5.0", Image: "registry.example.com/browser:1.5.0", DesiredSpec: "{}", Status: model.ReleaseStatusFailed, CreatedBy: 1}
	if err := s.CreateRelease(active); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(failed); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "browser.example.com", TLSEnabled: true, ServicePort: 80}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "browser-2", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ManagedByLabel: application.ManagedByValue, application.ApplicationNameLabel: app.Name, application.ReleaseLabel: "2"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "browser", Ready: true}}},
	})}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "\"status\":\"running\"") || !strings.Contains(response.Body.String(), "\"version\":\"1.4.0\"") || !strings.Contains(response.Body.String(), "https://browser.example.com") {
		t.Fatalf("unexpected workspace response: %d %s", response.Code, response.Body.String())
	}
}

func TestSyncApplicationEndpointsRemovesHTTPIngressForUDPService(t *testing.T) {
	_, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "udp.example.com", Path: "/", ServicePort: 443}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset()
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()
	handler := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	if err := handler.syncApplicationEndpoints(t.Context(), app, application.ServiceSpec{Port: 443}); err != nil {
		t.Fatalf("create TCP Ingress: %v", err)
	}
	if _, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected TCP ingress: %v", err)
	}
	if err := handler.syncApplicationEndpoints(t.Context(), app, application.ServiceSpec{Port: 443, Protocol: application.ServiceProtocolUDP}); err != nil {
		t.Fatalf("remove UDP ingress: %v", err)
	}
	if _, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("UDP Service must not retain an HTTP Ingress")
	}
}

func TestSyncApplicationEndpointsUsesFirstTCPPortForMultiPortService(t *testing.T) {
	_, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "api.example.com", Path: "/", ServicePort: 80}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset()
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()

	handler := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	service := application.ServiceSpec{Ports: []application.ServicePortSpec{
		{Name: "proxy", Port: 443, TargetPort: 443, Protocol: application.ServiceProtocolUDP},
		{Name: "api", Port: 8080, TargetPort: 8080, Protocol: application.ServiceProtocolTCP},
	}}
	if err := handler.syncApplicationEndpoints(t.Context(), app, service); err != nil {
		t.Fatalf("sync multi-port ingress: %v", err)
	}
	ingress, err := clientset.NetworkingV1().Ingresses(app.Environment.Namespace).Get(t.Context(), app.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if port := ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Number; port != 8080 {
		t.Fatalf("expected first TCP Service port, got %d", port)
	}
	endpoints, err := s.ListApplicationEndpoints(app.ID)
	if err != nil || endpoints[0].ServicePort != 8080 {
		t.Fatalf("expected endpoint to track TCP Service port, endpoints=%#v err=%v", endpoints, err)
	}
}

func TestApplicationHandlerGetReleaseIncludesLivePodRuntime(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	release := &model.Release{ApplicationID: app.ID, Sequence: 6, Image: "gcr.io/zenika-hub/alpine-chrome:124", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1, PodTrackingEnabled: true}
	if err := s.CreateRelease(release); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "browser-abc", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name, application.ReleaseLabel: "6"}},
		Spec:       corev1.PodSpec{NodeName: "worker-a"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "browser", RestartCount: 4, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}},
		}}},
	})}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/releases/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "\"tracking\":\"exact\"") || !strings.Contains(response.Body.String(), "CrashLoopBackOff") || !strings.Contains(response.Body.String(), "正常退出") {
		t.Fatalf("unexpected release runtime response: %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationHandlerGetReleaseMarksLegacyReleaseAsUntracked(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	release := &model.Release{ApplicationID: app.ID, Sequence: 1, Image: "nginx:1.27", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
	if err := s.CreateRelease(release); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/releases/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "legacy_untracked") {
		t.Fatalf("unexpected legacy release response: %d %s", response.Code, response.Body.String())
	}
}

func TestCreateRestartReleaseUsesCurrentTemplateAndSanitizesSecrets(t *testing.T) {
	_, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	key := []byte("01234567890123456789012345678901")

	currentTemplateSpec := application.ReleaseSpec{
		Image:         "ghcr.io/example/hysteria:template",
		Command:       []string{"hysteria", "server", "-c", "/etc/hysteria/config.yaml"},
		ContainerPort: 8443,
		Replicas:      1,
		Resources: application.ResourceSpec{
			RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi",
		},
		Secrets:  map[string]string{"API_TOKEN": ""},
		Service:  application.ServiceSpec{Port: 8443, TargetPort: 8443, Protocol: application.ServiceProtocolUDP, Type: application.ServiceTypeLoadBalancer},
		Endpoint: application.EndpointSpec{Exposure: application.ExposureCluster},
	}
	templateSnapshot, err := json.Marshal(application.SanitizeReleaseSpec(currentTemplateSpec))
	if err != nil {
		t.Fatal(err)
	}
	encryptedSecrets, err := crypto.Encrypt(key, `{"API_TOKEN":"template-secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{
		ApplicationID: app.ID, Name: "current", Enabled: true, Spec: string(templateSnapshot), EncryptedSecrets: encryptedSecrets,
	}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	active := &model.Release{
		ApplicationID: app.ID, Sequence: 4, Image: "not a valid image", Version: "2.12.1",
		DesiredSpec: `{"command":["legacy-command"],"secrets":{"API_TOKEN":""}}`, Status: model.ReleaseStatusSucceeded, CreatedBy: 1,
	}
	if err := s.CreateRelease(active); err != nil {
		t.Fatal(err)
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: app.Environment.Namespace},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	})}
	defer func() { K8s = originalK8s }()

	handler := NewApplicationHandler(s, key)
	release, err := handler.createRestartRelease(t.Context(), app, 9)
	if err != nil {
		t.Fatalf("create restart release: %v", err)
	}
	if release.Sequence != active.Sequence+1 || release.Image != active.Image || release.Version != active.Version {
		t.Fatalf("restart did not inherit active release version: %#v", release)
	}
	if release.TemplateID == nil || *release.TemplateID != template.ID || release.TemplateRevision != template.Revision {
		t.Fatalf("restart did not retain current template reference: %#v", release)
	}
	if strings.Contains(release.DesiredSpec, "template-secret") {
		t.Fatalf("restart snapshot leaked plaintext Secret: %s", release.DesiredSpec)
	}

	var snapshot application.ReleaseSpec
	if err := json.Unmarshal([]byte(release.DesiredSpec), &snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Join(snapshot.Command, " ") != strings.Join(currentTemplateSpec.Command, " ") || snapshot.Secrets["API_TOKEN"] != "" {
		t.Fatalf("restart did not rebuild sanitized snapshot from current template: %#v", snapshot)
	}
	resources, err := application.RenderResources(application.ApplicationContext{
		ProjectName: "runtime-project", EnvironmentName: "dev", Namespace: app.Environment.Namespace,
		ApplicationName: app.Name, ReleaseSequence: release.Sequence, WorkloadKind: app.WorkloadKind,
	}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(resources.Deployment.Spec.Template.Spec.Containers[0].Command, " "); got != strings.Join(currentTemplateSpec.Command, " ") {
		t.Fatalf("restart pod template command = %q, want %q", got, strings.Join(currentTemplateSpec.Command, " "))
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, getErr := s.GetRelease(release.ID)
		if getErr == nil && stored.Status == model.ReleaseStatusFailed {
			return // The invalid test image makes the asynchronous worker finish before restoring K8s.
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("restart release worker did not finish")
}

func createApplicationForReleaseRuntimeTest(t *testing.T, s *store.Store) *model.Application {
	t.Helper()
	project := &model.Project{Name: "runtime-project", OwnerID: 1}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "dev", Namespace: "runtime-dev"}
	if err := s.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	app := &model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "browser", WorkloadKind: "deployment", CreatedBy: 1}
	if err := s.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	app, err := s.GetApplication(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestIntegrationConfigMapReadsAndReplacesTemplateConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := app.SetCapabilities([]string{"config-editor"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	spec := application.ReleaseSpec{
		Image: "docker.io/example/app", ContainerPort: 8080, Replicas: 1,
		Resources:  application.ResourceSpec{RequestsCPU: "10m", RequestsMemory: "16Mi", LimitsCPU: "100m", LimitsMemory: "64Mi"},
		Config:     map[string]string{"config.yaml": "listen: :8443\n"},
		FileMounts: []application.FileMountSpec{{SourceType: application.FileMountSourceApplicationConfig, Key: "config.yaml", MountPath: "/etc/app/config.yaml", Managed: true}},
		Service:    application.ServiceSpec{Port: 8080, TargetPort: 8080, Protocol: application.ServiceProtocolTCP, Type: application.ServiceTypeClusterIP},
	}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(snapshot)}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	file := &model.ApplicationManagedFile{ApplicationID: app.ID, ResourceKind: application.FileMountSourceConfigMap, ResourceName: app.Name + "-config", Key: "config.yaml", MountPath: "/etc/app/config.yaml", CreatedBy: 1, Enabled: true}
	if err := s.CreateApplicationManagedFile(file); err != nil {
		t.Fatal(err)
	}

	h := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	claims := &auth.DelegationClaims{UserID: 1, ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "config-editor", Actions: []string{"configmap:read", "configmap:write"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Set("user_id", uint(1)); c.Next() })
	r.GET("/api/integrations/applications/:id/configmaps/:configMapID", h.IntegrationGetManagedConfigMap)
	r.PUT("/api/integrations/applications/:id/configmaps/:configMapID", h.IntegrationReplaceManagedConfigMap)

	response := serve(r, newJSONRequest(http.MethodGet, "/api/integrations/applications/1/configmaps/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"content":"listen: :8443\n"`) || !strings.Contains(response.Body.String(), `"version":1`) {
		t.Fatalf("unexpected ConfigMap read response: %d %s", response.Code, response.Body.String())
	}
	response = serve(r, newJSONRequest(http.MethodPut, "/api/integrations/applications/1/configmaps/1", gin.H{"content": "listen: :9443\n", "expected_revision": 1}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"version":2`) {
		t.Fatalf("unexpected ConfigMap replace response: %d %s", response.Code, response.Body.String())
	}
	updatedTemplate, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil || !strings.Contains(updatedTemplate.Spec, "listen: :9443") {
		t.Fatalf("template ConfigMap content was not updated: template=%#v err=%v", updatedTemplate, err)
	}
	response = serve(r, newJSONRequest(http.MethodPut, "/api/integrations/applications/1/configmaps/1", gin.H{"content": "stale", "expected_revision": 1}))
	if response.Code != http.StatusConflict {
		t.Fatalf("expected stale managed file update to fail, got %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationHandlerReleasesFromSelectedDeploymentTemplateByVersion(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	spec := gin.H{
		"image": "order-api", "container_port": 8080, "replicas": 1,
		"resources": gin.H{"requests_cpu": "100m", "requests_memory": "128Mi", "limits_cpu": "500m", "limits_memory": "512Mi"},
		"health":    gin.H{"readiness_enabled": false, "readiness_type": "http", "readiness_path": "/healthz", "liveness_enabled": false, "liveness_type": "http", "liveness_path": "/healthz"},
		"service":   gin.H{"port": 80, "target_port": 8080},
	}
	template := gin.H{"name": "标准上线", "enabled": true, "spec": spec}
	save := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", template))
	if save.Code != http.StatusOK {
		t.Fatalf("save template status = %d: %s", save.Code, save.Body.String())
	}
	loaded := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/deployment-templates", nil))
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), "标准上线") || !strings.Contains(loaded.Body.String(), "\"revision\":1") {
		t.Fatalf("unexpected template response: %s", loaded.Body.String())
	}
	canary := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates", gin.H{"name": "灰度配置", "enabled": true, "spec": spec}))
	if canary.Code != http.StatusOK {
		t.Fatalf("create second template status = %d: %s", canary.Code, canary.Body.String())
	}
	canaryID := responseID(t, canary.Body.Bytes())
	setDefault := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/deployment-templates/2/default", nil))
	if setDefault.Code != http.StatusOK {
		t.Fatalf("set default template status = %d: %s", setDefault.Code, setDefault.Body.String())
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})}
	defer func() { K8s = originalK8s }()
	release := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/releases", gin.H{"template_id": canaryID, "version": "1.2.3"}))
	if release.Code != http.StatusOK || !strings.Contains(release.Body.String(), "order-api:1.2.3") || !strings.Contains(release.Body.String(), "\"template_id\":2") {
		t.Fatalf("unexpected version release: %d %s", release.Code, release.Body.String())
	}
}

func TestPrepareReleaseImageVerificationUsesAppliedNodeMirror(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := &model.Server{Name: "worker-a", Host: "10.0.0.11", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := s.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	mirror := &model.NodeRegistryMirror{Name: "docker-hub", Registry: "docker.io", Endpoints: `["https://docker.1panel.live"]`, Enabled: true}
	if err := s.CreateNodeRegistryMirror(mirror); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirror.ID, ServerID: server.ID, Status: "success"}); err != nil {
		t.Fatal(err)
	}
	handler := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	spec := application.ReleaseSpec{Image: "zenika/alpine-chrome:124", NodeName: "worker-a"}
	if err := handler.prepareReleaseImageVerification(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.ImageVerificationEndpoint != "https://docker.1panel.live" {
		t.Fatalf("expected applied node mirror endpoint, got %+v", spec)
	}
}

func TestApplicationHandlerBindsDomainIndependentlyFromReleaseTemplate(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "api.example.com", EnvironmentID: 1, Namespace: "commerce-prod", CertificateName: "api-cert", TLSSecretName: "api-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "admin.example.com", EnvironmentID: 1, Namespace: "commerce-prod", CertificateName: "admin-cert", TLSSecretName: "admin-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}})}
	defer func() { K8s = originalK8s }()

	bind := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false}))
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), "api.example.com") {
		t.Fatalf("bind endpoint: %d %s", bind.Code, bind.Body.String())
	}
	firstID := responseID(t, bind.Body.Bytes())
	second := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 2, "path": "/console", "tls_enabled": false}))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "admin.example.com") {
		t.Fatalf("bind second endpoint: %d %s", second.Code, second.Body.String())
	}
	endpoints := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/endpoints", nil))
	if endpoints.Code != http.StatusOK || !strings.Contains(endpoints.Body.String(), "api.example.com") || !strings.Contains(endpoints.Body.String(), "admin.example.com") {
		t.Fatalf("list endpoints: %d %s", endpoints.Code, endpoints.Body.String())
	}
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/endpoints/"+strconv.Itoa(int(firstID)), gin.H{"domain_id": 1, "path": "/v2", "tls_enabled": false}))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "/v2") {
		t.Fatalf("update endpoint: %d %s", updated.Code, updated.Body.String())
	}
	remove := serve(r, newJSONRequest(http.MethodDelete, "/api/applications/1/endpoints/"+strconv.Itoa(int(firstID)), nil))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete endpoint: %d %s", remove.Code, remove.Body.String())
	}
}

func TestApplicationHandlerRejectsHTTPIngressBindingForUDPService(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "edge", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "edge-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "udp-server", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateManagedDomain(&model.ManagedDomain{Hostname: "edge.example.com", EnvironmentID: 1, Namespace: "edge-prod", CertificateName: "edge-cert", TLSSecretName: "edge-tls", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: 1, Sequence: 1, Image: "registry.example.com/udp-server:1.0.0", DesiredSpec: `{"service":{"port":443,"protocol":"UDP"}}`, Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "edge-prod"}})}
	defer func() { K8s = originalK8s }()

	response := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "UDP Service 不支持 HTTP Ingress") {
		t.Fatalf("expected UDP Ingress rejection, got %d %s", response.Code, response.Body.String())
	}
	metadata := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false, "ingress_enabled": false}))
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"ingress_enabled":false`) {
		t.Fatalf("expected metadata-only UDP binding, got %d %s", metadata.Code, metadata.Body.String())
	}
	duplicateMetadata := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/endpoints", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false, "ingress_enabled": false}))
	if duplicateMetadata.Code != http.StatusOK {
		t.Fatalf("metadata-only bindings should not conflict, got %d %s", duplicateMetadata.Code, duplicateMetadata.Body.String())
	}
}

func responseID(t *testing.T, body []byte) uint {
	t.Helper()
	var response struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response.Data.ID
}

func TestApplicationHandler_ProjectAndEnvironmentCRUD(t *testing.T) {
	r, s := setupApplicationRouter()
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = originalK8s }()

	createProject := serve(r, newJSONRequest(http.MethodPost, "/api/projects", gin.H{"name": "commerce", "description": "订单服务"}))
	if createProject.Code != http.StatusOK {
		t.Fatalf("create project status = %d", createProject.Code)
	}
	projectID := responseID(t, createProject.Body.Bytes())

	createEnvironment := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production", "namespace": "commerce-prod", "namespace_mode": "create"}))
	if createEnvironment.Code != http.StatusOK {
		t.Fatalf("create environment status = %d", createEnvironment.Code)
	}
	environmentID := responseID(t, createEnvironment.Body.Bytes())

	updateProject := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "commerce", "description": "订单核心服务"}))
	if updateProject.Code != http.StatusOK {
		t.Fatalf("update project status = %d", updateProject.Code)
	}
	updateEnvironment := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1/environments/1", gin.H{"name": "production", "namespace": "commerce-production", "namespace_mode": "create"}))
	if updateEnvironment.Code != http.StatusOK {
		t.Fatalf("update environment status = %d", updateEnvironment.Code)
	}

	projectList := serve(r, newJSONRequest(http.MethodGet, "/api/projects", nil))
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), "commerce-production") {
		t.Fatalf("project list should include environment relationship: %s", projectList.Body.String())
	}

	if err := s.CreateApplication(&model.Application{ProjectID: projectID, EnvironmentID: environmentID, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatalf("create application: %v", err)
	}
	deleteEnvironment := serve(r, newJSONRequest(http.MethodDelete, "/api/projects/1/environments/1", nil))
	if deleteEnvironment.Code != http.StatusConflict {
		t.Fatalf("delete referenced environment status = %d", deleteEnvironment.Code)
	}
	deleteProject := serve(r, newJSONRequest(http.MethodDelete, "/api/projects/1", nil))
	if deleteProject.Code != http.StatusConflict {
		t.Fatalf("delete referenced project status = %d", deleteProject.Code)
	}
}

func TestApplicationHandlerEnvironmentNamespaceBindAndSync(t *testing.T) {
	r, s := setupApplicationRouter()
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}

	bind := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production", "namespace": "existing", "namespace_mode": "bind"}))
	if bind.Code != http.StatusOK {
		t.Fatalf("bind environment status = %d: %s", bind.Code, bind.Body.String())
	}
	missingBind := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "staging", "namespace": "missing", "namespace_mode": "bind"}))
	if missingBind.Code != http.StatusBadRequest || !strings.Contains(missingBind.Body.String(), "无法绑定") {
		t.Fatalf("expected missing bind to fail: %s", missingBind.Body.String())
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "development", Namespace: "dev"}); err != nil {
		t.Fatal(err)
	}
	sync := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments/2/sync-namespace", nil))
	if sync.Code != http.StatusOK {
		t.Fatalf("sync namespace status = %d: %s", sync.Code, sync.Body.String())
	}
	if _, err := clientset.CoreV1().Namespaces().Get(t.Context(), "dev", metav1.GetOptions{}); err != nil {
		t.Fatalf("expected sync to create namespace: %v", err)
	}
}

func TestApplicationHandlerRejectsDuplicateSystemAndForeignNamespaceBindings(t *testing.T) {
	r, s := setupApplicationRouter()
	originalK8s := K8s
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared", Labels: map[string]string{"cylism.io/project-id": "1"}}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Labels: map[string]string{"cylism.io/project-id": "2"}}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
	)
	K8s = &k8sclient.Client{Clientset: clientset}
	defer func() { K8s = originalK8s }()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(&model.Project{Name: "payments", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}

	first := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production", "namespace": "shared", "namespace_mode": "bind"}))
	if first.Code != http.StatusOK {
		t.Fatalf("first bind: %d %s", first.Code, first.Body.String())
	}
	duplicate := serve(r, newJSONRequest(http.MethodPost, "/api/projects/2/environments", gin.H{"name": "production", "namespace": "shared", "namespace_mode": "bind"}))
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "已被项目") {
		t.Fatalf("duplicate bind: %d %s", duplicate.Code, duplicate.Body.String())
	}
	system := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "system", "namespace": "kube-system", "namespace_mode": "bind"}))
	if system.Code != http.StatusBadRequest || !strings.Contains(system.Body.String(), "系统命名空间") {
		t.Fatalf("system bind: %d %s", system.Code, system.Body.String())
	}
	foreign := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "foreign", "namespace": "foreign", "namespace_mode": "bind"}))
	if foreign.Code != http.StatusBadRequest || !strings.Contains(foreign.Body.String(), "已属于项目 2") {
		t.Fatalf("foreign bind: %d %s", foreign.Code, foreign.Body.String())
	}
}

func TestApplicationHandlerScopesWorkspaceApplicationsAndOverview(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}

	applications := serve(r, newJSONRequest(http.MethodGet, "/api/applications?project_id=1&environment_id=1", nil))
	if applications.Code != http.StatusOK || !strings.Contains(applications.Body.String(), "order-api") {
		t.Fatalf("scoped applications: %d %s", applications.Code, applications.Body.String())
	}
	overview := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1", nil))
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), "order-api") {
		t.Fatalf("workspace overview: %d %s", overview.Code, overview.Body.String())
	}
}

func TestApplicationHandlerRejectsInvalidKubernetesApplicationName(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "前端", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: 1, Name: "生产环境", Namespace: "frontend-prod"}); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodPost, "/api/applications", gin.H{"project_id": 1, "environment_id": 1, "name": "订单服务"}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "应用名称必须") {
		t.Fatalf("expected invalid application name error, got %d: %s", response.Code, response.Body.String())
	}
}

func TestApplicationHandler_ProjectDefaultRegistryMustBeAuthorized(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{Name: "commerce-harbor", Endpoint: "harbor.example.com", AuthType: "anonymous", Enabled: true}
	if err := s.CreateImageRegistry(registry, []uint{1}); err != nil {
		t.Fatal(err)
	}

	setDefault := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "commerce", "default_image_registry_id": registry.ID}))
	if setDefault.Code != http.StatusOK {
		t.Fatalf("set default registry status = %d: %s", setDefault.Code, setDefault.Body.String())
	}
	project, err := s.GetProject(1)
	if err != nil || project.DefaultImageRegistryID == nil || *project.DefaultImageRegistryID != registry.ID {
		t.Fatalf("expected default registry to be saved, got %+v err=%v", project, err)
	}

	other := &model.ImageRegistry{Name: "other-harbor", Endpoint: "other.example.com", AuthType: "anonymous", Enabled: true}
	if err := s.CreateImageRegistry(other, []uint{1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(&model.Project{Name: "payments", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	invalid := serve(r, newJSONRequest(http.MethodPut, "/api/projects/2", gin.H{"name": "payments", "default_image_registry_id": registry.ID}))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("default registry without project authorization status = %d: %s", invalid.Code, invalid.Body.String())
	}
}
