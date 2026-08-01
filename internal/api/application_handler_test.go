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
		applications.POST("", h.CreateApplication)
		applications.GET("/:id/deployment-templates", h.ListDeploymentTemplates)
		applications.POST("/:id/deployment-templates", h.CreateDeploymentTemplate)
		applications.GET("/:id/deployment-templates/:templateID", h.GetDeploymentTemplate)
		applications.PUT("/:id/deployment-templates/:templateID", h.UpdateDeploymentTemplate)
		applications.DELETE("/:id/deployment-templates/:templateID", h.DeleteDeploymentTemplate)
		applications.POST("/:id/deployment-templates/:templateID/default", h.SetDefaultDeploymentTemplate)
		applications.GET("/:id/endpoint", h.GetApplicationEndpoint)
		applications.PUT("/:id/endpoint", h.UpdateApplicationEndpoint)
		applications.DELETE("/:id/endpoint", h.DeleteApplicationEndpoint)
		applications.POST("/:id/releases", h.CreateRelease)
	}
	workspace := r.Group("/api/workspace")
	workspace.GET("/overview", h.WorkspaceOverview)
	return r, s
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
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "commerce-prod"}})}
	defer func() { K8s = originalK8s }()

	bind := serve(r, newJSONRequest(http.MethodPut, "/api/applications/1/endpoint", gin.H{"domain_id": 1, "path": "/", "tls_enabled": false}))
	if bind.Code != http.StatusOK || !strings.Contains(bind.Body.String(), "api.example.com") {
		t.Fatalf("bind endpoint: %d %s", bind.Code, bind.Body.String())
	}
	endpoint := serve(r, newJSONRequest(http.MethodGet, "/api/applications/1/endpoint", nil))
	if endpoint.Code != http.StatusOK || !strings.Contains(endpoint.Body.String(), "api.example.com") {
		t.Fatalf("get endpoint: %d %s", endpoint.Code, endpoint.Body.String())
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
