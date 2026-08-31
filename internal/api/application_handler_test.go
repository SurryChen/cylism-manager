package api

import (
	"encoding/json"
	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
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
