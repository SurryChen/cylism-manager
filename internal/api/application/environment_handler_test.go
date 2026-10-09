package applicationapi

import (
	"context"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
)

func TestApplicationHandlerSyncsNamespaceThroughEnvironmentCluster(t *testing.T) {
	platform := k8sfake.NewSimpleClientset()
	cluster := k8sfake.NewSimpleClientset()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	project := &model.Project{Name: "commerce", OwnerID: 1}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	environment := &model.Environment{ProjectID: project.ID, ClusterID: 2, Name: "production", Namespace: "commerce-prod"}
	if err := s.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(s, []byte("01234567890123456789012345678901"), NewKubernetesAdapter(&k8sclient.Client{Clientset: platform}))
	h.WithClusterClientFactory(func(_ context.Context, clusterID uint) (*k8sclient.Client, error) {
		if clusterID != 2 {
			t.Fatalf("unexpected cluster ID %d", clusterID)
		}
		return &k8sclient.Client{Clientset: cluster}, nil
	})
	r := gin.New()
	r.POST("/api/projects/:projectID/environments/:environmentID/sync-namespace", h.HandleEnvironment((*ApplicationHandler).SyncEnvironmentNamespace))
	response := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments/1/sync-namespace", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("sync namespace: %d %s", response.Code, response.Body.String())
	}
	if _, err := cluster.CoreV1().Namespaces().Get(t.Context(), environment.Namespace, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace missing from selected cluster: %v", err)
	}
	if _, err := platform.CoreV1().Namespaces().Get(t.Context(), environment.Namespace, metav1.GetOptions{}); err == nil {
		t.Fatal("namespace was created in the platform cluster")
	}
}

func TestApplicationHandler_ProjectAndEnvironmentCRUD(t *testing.T) {
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()})

	createProject := serve(r, newJSONRequest(http.MethodPost, "/api/projects", gin.H{"name": "电商", "slug": "commerce", "description": "订单服务"}))
	if createProject.Code != http.StatusOK {
		t.Fatalf("create project status = %d", createProject.Code)
	}
	projectID := responseID(t, createProject.Body.Bytes())

	createEnvironment := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production", "namespace": "ignored", "namespace_mode": "bind"}))
	if createEnvironment.Code != http.StatusOK {
		t.Fatalf("create environment status = %d", createEnvironment.Code)
	}
	environmentID := responseID(t, createEnvironment.Body.Bytes())
	if !strings.Contains(createEnvironment.Body.String(), "project-commerce-production") {
		t.Fatalf("expected generated namespace: %s", createEnvironment.Body.String())
	}

	updateProject := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "电商", "description": "订单核心服务"}))
	if updateProject.Code != http.StatusOK {
		t.Fatalf("update project status = %d", updateProject.Code)
	}
	updateEnvironment := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1/environments/1", gin.H{"name": "staging", "namespace": "ignored"}))
	if updateEnvironment.Code != http.StatusOK {
		t.Fatalf("update environment status = %d", updateEnvironment.Code)
	}

	projectList := serve(r, newJSONRequest(http.MethodGet, "/api/projects", nil))
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), "project-commerce-staging") {
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

func TestApplicationHandlerEnvironmentNamespaceAutoCreateAndSync(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}})
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
	if err := s.CreateProject(&model.Project{Name: "commerce", Slug: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}

	created := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production", "namespace": "existing", "namespace_mode": "bind"}))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "project-commerce-production") {
		t.Fatalf("auto create environment status = %d: %s", created.Code, created.Body.String())
	}
	if _, err := clientset.CoreV1().Namespaces().Get(t.Context(), "project-commerce-production", metav1.GetOptions{}); err != nil {
		t.Fatalf("expected generated namespace: %v", err)
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

func TestApplicationHandlerLegacyProjectNeedsSlugForNewEnvironment(t *testing.T) {
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()})
	if err := s.CreateProject(&model.Project{Name: "旧项目", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	missing := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production"}))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "设置项目英文名") {
		t.Fatalf("missing project slug: %d %s", missing.Code, missing.Body.String())
	}
	updated := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "旧项目", "slug": "legacy"}))
	if updated.Code != http.StatusOK {
		t.Fatalf("set project slug: %d %s", updated.Code, updated.Body.String())
	}
	created := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production"}))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "project-legacy-production") {
		t.Fatalf("create environment: %d %s", created.Code, created.Body.String())
	}
}

func TestApplicationHandlerRejectsConflictingGeneratedNamespaces(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "project-commerce-foreign", Labels: map[string]string{"cylism.io/project-id": "2"}}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
	)
	r, s := setupApplicationRouter(&k8sclient.Client{Clientset: clientset})
	if err := s.CreateProject(&model.Project{Name: "commerce", Slug: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(&model.Project{Name: "payments", Slug: "commerce-production", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}

	first := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "production-dev"}))
	if first.Code != http.StatusOK {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	duplicate := serve(r, newJSONRequest(http.MethodPost, "/api/projects/2/environments", gin.H{"name": "dev"}))
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "已被项目") {
		t.Fatalf("duplicate generated namespace: %d %s", duplicate.Code, duplicate.Body.String())
	}
	invalid := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "生产环境"}))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "环境名称只能") {
		t.Fatalf("invalid environment name: %d %s", invalid.Code, invalid.Body.String())
	}
	foreign := serve(r, newJSONRequest(http.MethodPost, "/api/projects/1/environments", gin.H{"name": "foreign"}))
	if foreign.Code != http.StatusBadRequest || !strings.Contains(foreign.Body.String(), "已存在") {
		t.Fatalf("existing foreign namespace: %d %s", foreign.Code, foreign.Body.String())
	}
}
