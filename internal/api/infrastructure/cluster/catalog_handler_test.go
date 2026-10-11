package cluster

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestCatalogAgentStatusSeparatesMissingControlPlaneAndConnectionFailure(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.Cluster{Name: "测试集群", Distribution: "k3s", Status: "draft"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewCatalogHandler(st)
	r := gin.New()
	r.GET("/api/clusters/:id/agent-status", handler.AgentStatus)
	path := "/api/clusters/" + strconv.FormatUint(uint64(cluster.ID), 10) + "/agent-status"
	response := serve(r, newJSONRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"no_control_plane"`) {
		t.Fatalf("no control plane = %d %s", response.Code, response.Body.String())
	}
	serverID := uint(7)
	if err := st.DB().Model(&cluster).Update("control_server_id", serverID).Error; err != nil {
		t.Fatal(err)
	}
	handler.WithAgentClientFactory(func(context.Context, uint) (*k8s.Client, error) { return nil, errors.New("访问证书缺失") })
	response = serve(r, newJSONRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"certificate_error"`) {
		t.Fatalf("certificate error = %d %s", response.Code, response.Body.String())
	}
	clientset := k8sfake.NewSimpleClientset()
	clientset.PrependReactor("list", "namespaces", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("dial tcp 127.0.0.1:9443: connect: connection refused")
	})
	handler.WithAgentClientFactory(func(context.Context, uint) (*k8s.Client, error) { return &k8s.Client{Clientset: clientset}, nil })
	response = serve(r, newJSONRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"unreachable"`) {
		t.Fatalf("unreachable = %d %s", response.Code, response.Body.String())
	}
}

type failingCatalog struct{}

func (failingCatalog) ListClusters() ([]model.Cluster, error) {
	return nil, errors.New("database unavailable")
}
func (failingCatalog) GetCluster(uint) (*model.Cluster, error) {
	return nil, errors.New("database unavailable")
}
func (failingCatalog) GetClusterByName(string) (*model.Cluster, error) {
	return nil, errors.New("database unavailable")
}
func (failingCatalog) CreateCluster(*model.Cluster) error {
	return errors.New("database unavailable")
}
func (failingCatalog) UpdateClusterName(uint, string) (*model.Cluster, error) {
	return nil, errors.New("database unavailable")
}
func (failingCatalog) DeleteCluster(uint) error { return errors.New("database unavailable") }

func TestCatalogListReturnsStoredClusters(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Create(&model.Cluster{Name: "生产集群", Distribution: "k3s", NetworkMode: "tailscale", Status: "draft"}).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/api/clusters", NewCatalogHandler(st).List)
	response := serve(r, newJSONRequest(http.MethodGet, "/api/clusters", nil))
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"code":0`, `"name":"生产集群"`, `"distribution":"k3s"`, `"network_mode":"tailscale"`, `"status":"draft"`) {
		t.Fatalf("cluster list = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogListHandlesStorageFailure(t *testing.T) {
	r := gin.New()
	r.GET("/api/clusters", NewCatalogHandler(failingCatalog{}).List)
	response := serve(r, newJSONRequest(http.MethodGet, "/api/clusters", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "加载集群列表失败") {
		t.Fatalf("cluster list error = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogCreateAndGetLogicalCluster(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	handler := NewCatalogHandler(st)
	r.POST("/api/clusters", handler.Create)
	r.GET("/api/clusters/:id", handler.Get)

	response := serve(r, newJSONRequest(http.MethodPost, "/api/clusters", map[string]any{
		"name": "  生产集群  ", "distribution": "k3s", "status": "ready",
	}))
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"name":"生产集群"`, `"distribution":"k3s"`, `"status":"draft"`) {
		t.Fatalf("create cluster = %d %s", response.Code, response.Body.String())
	}
	stored, err := st.GetCluster(1)
	if err != nil || stored.Name != "生产集群" || stored.Status != "draft" {
		t.Fatalf("stored logical cluster = %#v, err=%v", stored, err)
	}

	response = serve(r, newJSONRequest(http.MethodGet, "/api/clusters/1", nil))
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"name":"生产集群"`, `"network_mode":"public"`, `"status":"draft"`) {
		t.Fatalf("get cluster = %d %s", response.Code, response.Body.String())
	}
	response = serve(r, newJSONRequest(http.MethodPost, "/api/clusters", map[string]any{"name": "生产集群", "distribution": "k3s"}))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "集群名称已存在") {
		t.Fatalf("duplicate cluster = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogRejectsInvalidNameAndMissingCluster(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	handler := NewCatalogHandler(st)
	r.POST("/api/clusters", handler.Create)
	r.GET("/api/clusters/:id", handler.Get)
	for _, name := range []string{"  ", strings.Repeat("a", 129)} {
		response := serve(r, newJSONRequest(http.MethodPost, "/api/clusters", map[string]any{"name": name, "distribution": "k3s"}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("name %q = %d %s", name, response.Code, response.Body.String())
		}
	}
	response := serve(r, newJSONRequest(http.MethodGet, "/api/clusters/99", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing cluster = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogCreateRequiresSupportedDistribution(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/api/clusters", NewCatalogHandler(st).Create)
	for _, item := range []struct {
		distribution string
		message      string
	}{
		{"", "请选择集群类型"},
		{"kubernetes", "K8s 暂未开放"},
		{"unknown", "集群类型不支持"},
	} {
		response := serve(r, newJSONRequest(http.MethodPost, "/api/clusters", map[string]any{"name": "测试集群", "distribution": item.distribution}))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), item.message) {
			t.Fatalf("distribution %q = %d %s", item.distribution, response.Code, response.Body.String())
		}
	}
}

func TestCatalogUpdateAndDeleteRequiresNoNodes(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.Cluster{Name: "旧名称", Distribution: "k3s", Status: "draft"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewCatalogHandler(st)
	r.PUT("/api/clusters/:id", h.Update)
	r.DELETE("/api/clusters/:id", h.Delete)
	response := serve(r, newJSONRequest(http.MethodPut, "/api/clusters/1", map[string]any{"name": "  新名称  ", "distribution": "kubernetes", "status": "ready"}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"新名称"`) {
		t.Fatalf("update = %d %s", response.Code, response.Body.String())
	}
	stored, err := st.GetCluster(cluster.ID)
	if err != nil || stored.Distribution != "k3s" || stored.Status != "draft" {
		t.Fatalf("update changed fixed fields = %#v, %v", stored, err)
	}
	server := model.Server{Name: "节点", Host: "10.0.0.1", ClusterID: &cluster.ID, ClusterRole: "worker"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	response = serve(r, newJSONRequest(http.MethodDelete, "/api/clusters/1", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "先卸载") {
		t.Fatalf("delete with node = %d %s", response.Code, response.Body.String())
	}
	if err := st.DB().Model(&server).Updates(map[string]any{"cluster_id": nil, "cluster_role": ""}).Error; err != nil {
		t.Fatal(err)
	}
	response = serve(r, newJSONRequest(http.MethodDelete, "/api/clusters/1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete empty = %d %s", response.Code, response.Body.String())
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
