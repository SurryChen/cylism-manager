package api

import (
	"encoding/json"
	"fmt"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
	"time"
)

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
	// The restart operation runs its release worker asynchronously. A plain
	// ":memory:" SQLite DSN creates one database per connection, so the worker
	// can observe an empty schema when it obtains a different pooled connection.
	// Use a shared in-memory database for this test so all connections see the
	// same schema while keeping the test isolated from other cases.
	dsn := fmt.Sprintf("file:restart-release-%d?mode=memory&cache=shared", time.Now().UnixNano())
	s, err := store.New(dsn)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
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
