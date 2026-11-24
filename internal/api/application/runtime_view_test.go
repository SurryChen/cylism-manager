package applicationapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/application"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestApplicationEndpointURLSkipsRecordedDomains(t *testing.T) {
	app := model.Application{Endpoints: []model.ApplicationEndpoint{
		{Domain: "record.example.com", IngressMode: "metadata", Path: "/"},
		{Domain: "web.example.com", Path: "/app", TLSEnabled: true},
	}}
	if got := applicationEndpointURL(app); got != "https://web.example.com/app" {
		t.Fatalf("unexpected endpoint URL: %q", got)
	}
	app.Endpoints = app.Endpoints[:1]
	if got := applicationEndpointURL(app); got != "" {
		t.Fatalf("record-only domain should not expose a URL: %q", got)
	}
}

func TestApplicationHandlerDiscoversCapabilityAndSanitizedRuntime(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset()
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
	app := createApplicationForReleaseRuntimeTest(t, s)
	if _, err := s.ReplaceApplicationCapabilities(app.ID, []string{"hysteria2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 4, Version: "2.12.1", Image: "ghcr.io/example/hysteria:2.12.1", DesiredSpec: `{"secrets":{"api":"private"}}`, Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Services(app.Environment.Namespace).Create(t.Context(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Environment.Namespace},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{{Name: "proxy", Port: 8443, TargetPort: intstr.FromInt32(8443), Protocol: corev1.ProtocolUDP}, {Name: "traffic-api", Port: 10001, TargetPort: intstr.FromInt32(10001), Protocol: corev1.ProtocolTCP}}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.20"}}}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.AppsV1().Deployments(app.Environment.Namespace).Create(t.Context(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Environment.Namespace}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, pod := range []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "hysteria-ready", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name}}, Spec: corev1.PodSpec{NodeName: "worker-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "hysteria", Ready: true}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "hysteria-pending", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name}}, Spec: corev1.PodSpec{NodeName: "worker-b"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
	} {
		if _, err := clientset.CoreV1().Pods(app.Environment.Namespace).Create(t.Context(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

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

func TestWorkspaceOverviewIncludesApplicationRuntimeSummary(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
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
	if _, err := clientset.CoreV1().Pods(app.Environment.Namespace).Create(t.Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "browser-2", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ManagedByLabel: application.ManagedByValue, application.ApplicationNameLabel: app.Name, application.ReleaseLabel: "2"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "browser", Ready: true}}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "\"status\":\"running\"") || !strings.Contains(response.Body.String(), "\"version\":\"1.4.0\"") || !strings.Contains(response.Body.String(), "https://browser.example.com") || !strings.Contains(response.Body.String(), "\"releases\":[{") {
		t.Fatalf("unexpected workspace response: %d %s", response.Code, response.Body.String())
	}
}

func TestWorkspaceOverviewUsesPodsFromFailedReleaseThatLaterBecameReady(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
	app := createApplicationForReleaseRuntimeTest(t, s)
	failed := &model.Release{ApplicationID: app.ID, Sequence: 2, Version: "2.0.0", Image: "example.com/browser:2.0.0", DesiredSpec: "{}", Status: model.ReleaseStatusFailed, CreatedBy: 1}
	if err := s.CreateRelease(failed); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Pods(app.Environment.Namespace).Create(t.Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "browser-2", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ManagedByLabel: application.ManagedByValue, application.ApplicationNameLabel: app.Name, application.ReleaseLabel: "2"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "browser", Ready: true}}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("workspace response: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Applications []struct {
				ActiveRelease *struct {
					Sequence uint   `json:"sequence"`
					Version  string `json:"version"`
				} `json:"active_release"`
				LatestRelease *struct {
					Status string `json:"status"`
				} `json:"latest_release"`
				Runtime struct {
					Status    string `json:"status"`
					ReadyPods int32  `json:"ready_pods"`
					TotalPods int32  `json:"total_pods"`
				} `json:"runtime"`
			} `json:"applications"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Applications) != 1 {
		t.Fatalf("unexpected applications: %s", response.Body.String())
	}
	info := payload.Data.Applications[0]
	if info.Runtime.Status != "running" || info.Runtime.ReadyPods != 1 || info.Runtime.TotalPods != 1 || info.ActiveRelease == nil || info.ActiveRelease.Sequence != 2 || info.ActiveRelease.Version != "2.0.0" || info.LatestRelease == nil || info.LatestRelease.Status != model.ReleaseStatusFailed {
		t.Fatalf("workspace should show live pods and historical release failure: %s", response.Body.String())
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
