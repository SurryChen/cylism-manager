package applicationapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/application"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestWorkspacePagesRetainFullSummaryAndScope(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: client})
	app := createApplicationForReleaseRuntimeTest(t, s)
	for i := 2; i <= 24; i++ {
		item := model.Application{ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, Name: fmt.Sprintf("app-%02d", i), CreatedAt: app.CreatedAt}
		if err := s.CreateApplication(&item); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.CoreV1().Pods(app.Environment.Namespace).Create(t.Context(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: app.Environment.Namespace, Labels: map[string]string{application.ApplicationNameLabel: app.Name, application.ReleaseLabel: "1", application.ManagedByLabel: application.ManagedByValue}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 1, Status: model.ReleaseStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	foreign := model.Environment{ProjectID: app.ProjectID, Name: "other", Namespace: "other"}
	if err := s.CreateEnvironment(&foreign); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplication(&model.Application{ProjectID: app.ProjectID, EnvironmentID: foreign.ID, Name: "foreign"}); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"runtime", "config"} {
		response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1&view="+view+"&limit=10&offset=10", nil))
		var payload struct {
			Data struct {
				Items             []workspaceApplicationInfo
				Total             int
				ApplicationsTotal int      `json:"applications_total"`
				ReadyApplications int      `json:"ready_applications"`
				Names             []string `json:"application_names"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		d := payload.Data
		if response.Code != 200 || d.Total != 24 || len(d.Items) != 10 || d.ApplicationsTotal != 24 || len(d.Names) != 24 || d.ReadyApplications != 1 {
			t.Fatalf("%s: %s", view, response.Body.String())
		}
		if d.Items[0].ID != 14 || d.Items[9].ID != 5 {
			t.Fatalf("unstable page order: %s", response.Body.String())
		}
	}
	response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1&limit=10&view=invalid", nil))
	if response.Code != 400 {
		t.Fatalf("invalid view accepted: %s", response.Body.String())
	}
}

func TestWorkspaceAccessPagesFlattenEndpointsAndKeepEmptyApps(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	for i := 0; i < 12; i++ {
		if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, Domain: fmt.Sprintf("app-%d.example.com", i), IngressMode: "metadata", Path: "/", ServicePort: 80}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateApplication(&model.Application{ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, Name: "no-endpoints"}); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/overview?project_id=1&environment_id=1&view=access&limit=10&offset=10", nil))
	var payload struct {
		Data struct {
			Items []workspaceEndpointRow
			Total int
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || payload.Data.Total != 13 || len(payload.Data.Items) != 3 {
		t.Fatalf("unexpected endpoint page: %s", response.Body.String())
	}
	for _, row := range payload.Data.Items {
		if row.Endpoint == nil || row.Endpoint.IngressEnabled {
			t.Fatalf("invalid endpoint DTO: %s", response.Body.String())
		}
	}
}
