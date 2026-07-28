package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupApplicationRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewApplicationHandler(s)
	projects := r.Group("/api/projects")
	{
		projects.GET("", h.ListProjects)
		projects.POST("", h.CreateProject)
		projects.PUT("/:projectID", h.UpdateProject)
		projects.DELETE("/:projectID", h.DeleteProject)
		projects.GET("/:projectID/environments", h.ListEnvironments)
		projects.POST("/:projectID/environments", h.CreateEnvironment)
		projects.PUT("/:projectID/environments/:environmentID", h.UpdateEnvironment)
		projects.POST("/:projectID/environments/:environmentID/sync-namespace", h.SyncEnvironmentNamespace)
		projects.DELETE("/:projectID/environments/:environmentID", h.DeleteEnvironment)
	}
	return r, s
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
