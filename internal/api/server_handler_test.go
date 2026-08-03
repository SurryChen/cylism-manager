package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupServerRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	encKey := make([]byte, 32)
	h := NewServerHandler(s, encKey)
	servers := r.Group("/api/servers")
	{
		servers.POST("", h.Create)
		servers.GET("", h.List)
		servers.GET("/resource-stats", h.ResourceStats)
		servers.GET("/:id", h.Get)
		servers.PUT("/:id", h.Update)
		servers.DELETE("/:id", h.Delete)
		servers.POST("/:id/unbind", h.Unbind)
		servers.POST("/:id/probe", h.Probe)
		servers.POST("/:id/precheck", h.Precheck)
	}
	return r, s
}

func TestServerHandlerResourceStatsReturnsPerServerSuccessAndFailure(t *testing.T) {
	r, s := setupServerRouter()
	if err := s.CreateServer(&model.Server{Name: "ready", Host: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateServer(&model.Server{Name: "offline", Host: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	h := NewServerHandler(s, make([]byte, 32))
	h.statsCollector = func(server *model.Server) (gin.H, error) {
		if server.Name == "offline" {
			return nil, fmt.Errorf("SSH connection timed out")
		}
		return gin.H{"cpu_percent": 42.5, "memory_total_mb": 1024}, nil
	}
	r = gin.New()
	r.GET("/api/servers/resource-stats", h.ResourceStats)

	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers/resource-stats", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"ready"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"unreachable"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"server_id"`)) {
		t.Fatalf("unexpected resource stats response: %d %s", w.Code, w.Body.String())
	}
}

func TestParseServerStats(t *testing.T) {
	stats := parseServerStats("CPU: 42.5\nCPU_CORES: 4\nMEM: 8192 4096 3072\nDISK: 40 12 28 30%\nLOAD: 1.25 0.75 0.50\nUP: 3 days")
	if stats["cpu_percent"] != 42.5 || stats["cpu_cores"] != 4 || stats["memory_total_mb"] != 8192 || stats["disk_used_gb"] != 12 || stats["load_1m"] != 1.25 || stats["uptime"] != "3 days" {
		t.Fatalf("unexpected parsed stats: %#v", stats)
	}
}

func newJSONRequest(method, path string, body interface{}) *http.Request {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestServerHandler_ProbeNotFound(t *testing.T) {
	r, _ := setupServerRouter()
	req := newJSONRequest("POST", "/api/servers/999/probe", nil)
	w := serve(r, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServerHandler_PrecheckNotFound(t *testing.T) {
	r, _ := setupServerRouter()
	req := newJSONRequest("POST", "/api/servers/999/precheck", nil)
	w := serve(r, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServerHandlerListUnbindsServerWhoseNodeWasRemovedExternally(t *testing.T) {
	original := K8s
	defer func() { K8s = original }()
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}

	r, s := setupServerRouter()
	server := &model.Server{Name: "worker", Host: "10.0.0.2", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := s.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	incomplete := &model.Server{Name: "legacy worker", Host: "10.0.0.3", ClusterRole: "worker"}
	if err := s.CreateServer(incomplete); err != nil {
		t.Fatal(err)
	}

	w := serve(r, newJSONRequest(http.MethodGet, "/api/servers", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"cluster_role":""`)) {
		t.Fatalf("expected response to contain an unbound server, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := s.GetServer(server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClusterRole != "" || updated.K8sNodeName != "" {
		t.Fatalf("expected persisted server unbinding, got %#v", updated)
	}
	updatedIncomplete, err := s.GetServer(incomplete.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedIncomplete.ClusterRole != "" || updatedIncomplete.K8sNodeName != "" {
		t.Fatalf("expected incomplete binding to be cleared, got %#v", updatedIncomplete)
	}
}

func TestServerHandlerUnbindClearsOnlyClusterAssociation(t *testing.T) {
	r, s := setupServerRouter()
	server := &model.Server{Name: "worker", Host: "10.0.0.2", ClusterRole: "worker", K8sNodeName: "worker-a"}
	if err := s.CreateServer(server); err != nil {
		t.Fatal(err)
	}

	w := serve(r, newJSONRequest(http.MethodPost, "/api/servers/1/unbind", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("已解除集群绑定")) {
		t.Fatalf("expected unbind success, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := s.GetServer(server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClusterRole != "" || updated.K8sNodeName != "" || updated.Host != server.Host {
		t.Fatalf("expected only cluster association to be cleared, got %#v", updated)
	}
}
