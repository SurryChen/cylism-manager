package cluster

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type fakeNodes struct{}

func (fakeNodes) ListNodeInfosContext(context.Context) ([]k8s.NodeInfo, error) {
	return []k8s.NodeInfo{{Name: "worker-a", Ready: true}}, nil
}
func (fakeNodes) GetNodeLabelsContext(context.Context, string) (*k8s.NodeLabels, error) {
	return &k8s.NodeLabels{Name: "worker-a", Labels: map[string]string{}}, nil
}
func (fakeNodes) UpdateNodeLabelsContext(context.Context, string, map[string]string, []string) (*k8s.NodeLabels, error) {
	return &k8s.NodeLabels{Name: "worker-a", Labels: map[string]string{"team": "platform"}}, nil
}
func (fakeNodes) DrainPlanContext(context.Context, string) (*k8s.DrainPlan, error) {
	return &k8s.DrainPlan{NodeName: "worker-a"}, nil
}
func (fakeNodes) DrainNodeContext(context.Context, string, k8s.DrainOptions) (*k8s.DrainResult, error) {
	return &k8s.DrainResult{Plan: &k8s.DrainPlan{NodeName: "worker-a"}}, nil
}
func (fakeNodes) ForceDrainNodeContext(context.Context, string, k8s.ForceDrainOptions) (*k8s.DrainResult, error) {
	return &k8s.DrainResult{Forced: true}, nil
}
func (fakeNodes) RejoinNodeContext(context.Context, string) (*k8s.NodeInfo, error) {
	return &k8s.NodeInfo{Name: "worker-a"}, nil
}
func (fakeNodes) NodeRemovalCheckContext(context.Context, string) (*k8s.NodeRemovalCheck, error) {
	return &k8s.NodeRemovalCheck{NodeName: "worker-a", CanRemove: true}, nil
}
func (fakeNodes) DeleteNodeContext(context.Context, string) error { return nil }

func newClusterTestRouter(t *testing.T) (*gin.Engine, *store.Store) {
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

func TestNodeAdapterKeepsForceDrainConfirmationAtServiceBoundary(t *testing.T) {
	r, _ := newClusterTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/nodes/worker-a/force-drain", bytes.NewBufferString(`{"acknowledge_risk":false,"confirm_node_name":"worker-a"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("请确认风险")) {
		t.Fatalf("force-drain response = %d %s", response.Code, response.Body.String())
	}
}
