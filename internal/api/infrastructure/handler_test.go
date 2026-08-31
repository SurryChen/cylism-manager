package infrastructure

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestInfrastructureAdaptersKeepInjectedHandlers(t *testing.T) {
	_ = NewNetworkHandler(nil, NetworkHandler{})
}

type fakeNodes struct{}

func (fakeNodes) ListNodeInfos() ([]k8s.NodeInfo, error) {
	return []k8s.NodeInfo{{Name: "worker-a", Ready: true}}, nil
}
func (fakeNodes) GetNodeLabels(string) (*k8s.NodeLabels, error) {
	return &k8s.NodeLabels{Name: "worker-a", Labels: map[string]string{}}, nil
}
func (fakeNodes) UpdateNodeLabels(string, map[string]string, []string) (*k8s.NodeLabels, error) {
	return &k8s.NodeLabels{Name: "worker-a", Labels: map[string]string{"team": "platform"}}, nil
}
func (fakeNodes) DrainPlan(string) (*k8s.DrainPlan, error) {
	return &k8s.DrainPlan{NodeName: "worker-a"}, nil
}
func (fakeNodes) DrainNode(string, k8s.DrainOptions) (*k8s.DrainResult, error) {
	return &k8s.DrainResult{Plan: &k8s.DrainPlan{NodeName: "worker-a"}}, nil
}
func (fakeNodes) ForceDrainNode(string, k8s.ForceDrainOptions) (*k8s.DrainResult, error) {
	return &k8s.DrainResult{Forced: true}, nil
}
func (fakeNodes) RejoinNode(string) (*k8s.NodeInfo, error) {
	return &k8s.NodeInfo{Name: "worker-a"}, nil
}
func (fakeNodes) NodeRemovalCheck(string) (*k8s.NodeRemovalCheck, error) {
	return &k8s.NodeRemovalCheck{NodeName: "worker-a", CanRemove: true}, nil
}
func (fakeNodes) DeleteNode(string) error { return nil }

func newInfrastructureTestRouter(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := cluster.NewService(st, fakeNodes{})
	r := gin.New()
	servers := NewServerHandler(make([]byte, 32), service)
	nodes := NewNodeHandler(service)
	r.POST("/api/servers", servers.Create)
	r.GET("/api/servers", servers.List)
	r.POST("/api/servers/:id/unbind", servers.Unbind)
	r.PATCH("/api/nodes/:id/labels", nodes.UpdateLabels)
	r.POST("/api/nodes/:id/force-drain", nodes.ForceDrainNode)
	return r, st
}

func TestServerLifecycleAdapterUsesUnifiedResponse(t *testing.T) {
	r, st := newInfrastructureTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBufferString(`{"name":"worker","host":"10.0.0.11"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"code":0`)) {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/servers", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"worker"`)) {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}

	serverID := uint(1)
	server, err := st.GetServer(serverID)
	if err != nil || server.Name != "worker" {
		t.Fatalf("stored server = %#v, err=%v", server, err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/servers/1/unbind", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("已解除集群绑定")) {
		t.Fatalf("unbind response = %d %s", response.Code, response.Body.String())
	}
}

func TestNodeAdapterKeepsForceDrainConfirmationAtServiceBoundary(t *testing.T) {
	r, _ := newInfrastructureTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/nodes/worker-a/force-drain", bytes.NewBufferString(`{"acknowledge_risk":false,"confirm_node_name":"worker-a"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("请确认风险")) {
		t.Fatalf("force-drain response = %d %s", response.Code, response.Body.String())
	}
}
