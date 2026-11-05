package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	migration "github.com/cylism/cylism-manager/internal/service/observability/migration"
	"github.com/gin-gonic/gin"
)

type migrationRepository struct {
	items []model.ObservabilityMigration
}

func (r *migrationRepository) ListServers() ([]model.Server, error) {
	id := uint(7)
	return []model.Server{{ClusterID: &id, K8sNodeName: "source"}, {ClusterID: &id, K8sNodeName: "target"}}, nil
}
func (r *migrationRepository) CreateObservabilityMigration(item *model.ObservabilityMigration) error {
	item.ID = 1
	item.TaskID = 20
	return nil
}
func (*migrationRepository) SaveObservabilityMigration(*model.ObservabilityMigration, string, bool) error {
	return nil
}
func (r *migrationRepository) ListObservabilityMigrations(uint, string) ([]model.ObservabilityMigration, error) {
	return r.items, nil
}
func (*migrationRepository) ActiveObservabilityMigrations() ([]model.ObservabilityMigration, error) {
	return nil, nil
}
func (r *migrationRepository) GetObservabilityMigration(id uint) (*model.ObservabilityMigration, error) {
	for _, item := range r.items {
		if item.ID == id {
			return &item, nil
		}
	}
	return nil, context.Canceled
}
func (r *migrationRepository) SetObservabilityMigrationControl(id uint, control, _ string) error {
	for i := range r.items {
		if r.items[i].ID == id {
			r.items[i].Control = control
			return nil
		}
	}
	return context.Canceled
}
func (*migrationRepository) UpdateObservabilityMigrationProgress(uint, int64, int64, int64) error {
	return nil
}

type migrationAdapter struct{ migration.Adapter }

func (migrationAdapter) ComponentStorageSnapshot(context.Context, string) (*k8s.ComponentStorageSnapshot, error) {
	return &k8s.ComponentStorageSnapshot{Component: k8s.InfrastructureLoki, Node: "source", PVC: "data"}, nil
}
func (migrationAdapter) ValidateComponentMigrationNode(context.Context, string) error { return nil }

// Stop before any mutation in this HTTP contract test; execution is tested in Service.
func (migrationAdapter) LockComponentMigration(context.Context, string, uint) error {
	return context.Canceled
}
func TestComponentMigrationHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &migrationRepository{items: []model.ObservabilityMigration{{ID: 1, ClusterID: 7, Component: k8s.InfrastructureLoki, Snapshot: "must not leak", Status: "copying"}}}
	svc := migration.New(repo, func(context.Context, uint) (migration.Adapter, error) { return migrationAdapter{}, nil }, nil)
	h := (&LoggingHandler{}).WithMigrations(svc, 7)
	router := gin.New()
	router.GET("/tasks", h.ListNodeMigrations)
	router.POST("/tasks", h.MigrateNode)
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "must not leak") {
		t.Fatal(get.Code, get.Body.String())
	}
	post := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"target_node_name":"target"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(post, req)
	if post.Code != http.StatusAccepted {
		t.Fatal(post.Code, post.Body.String())
	}
	var response struct {
		Data model.ObservabilityMigration `json:"data"`
	}
	if err := json.Unmarshal(post.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.TaskID != 20 || response.Data.ClusterID != 7 || response.Data.Status != "pending" {
		t.Fatalf("not a durable async task: %+v", response)
	}
	if err := componentMigrationMutable(svc, 7, k8s.InfrastructureLoki); err == nil {
		t.Fatal("active migration did not block settings")
	}
	repo.items[0].Status = "succeeded"
	if err := componentMigrationMutable(svc, 7, k8s.InfrastructureLoki); err != nil {
		t.Fatal(err)
	}
}
func TestComponentMigrationRequiresExplicitCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &MonitoringHandler{}
	router.POST("/tasks", h.MigrateNode)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"target_node_name":"target"}`)))
	if result.Code != http.StatusBadRequest {
		t.Fatal(result.Code)
	}
}
