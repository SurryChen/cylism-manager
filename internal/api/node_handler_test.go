package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setupNodeRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewNodeHandler(s, nil)
	nodes := r.Group("/api/nodes")
	{
		nodes.GET("", h.ListNode)
		nodes.GET("/:id/drain-plan", h.DrainPlan)
		nodes.GET("/:id/removal-check", h.RemovalCheck)
		nodes.POST("/:id/add", h.AddNode)
		nodes.POST("/:id/drain", h.DrainNode)
		nodes.POST("/:id/force-drain", h.ForceDrainNode)
		nodes.DELETE("/:id", h.RemoveNode)
	}
	return r, s
}

func TestNodeHandler_ListNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_AddNode(t *testing.T) {
	r, s := setupNodeRouter()
	s.DB().Exec("INSERT INTO servers (name, host, ssh_auth_type, ssh_user, ssh_port) VALUES ('test', '1.1.1.1', 'password', 'root', 22)")

	req := httptest.NewRequest(http.MethodPost, "/api/nodes/1/add", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_AddNodeNotFound(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/999/add", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestNodeHandler_DrainNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/web-01/drain", strings.NewReader(`{"delete_empty_dir_data":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_DrainPlan(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes/web-01/drain-plan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNodeHandler_ForceDrainRequiresConfirmationAndRejectsHealthyNode(t *testing.T) {
	original := K8s
	defer func() { K8s = original }()
	failedAt := metav1.NewTime(time.Now().Add(-6 * time.Minute))
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, LastHeartbeatTime: failedAt}}},
	})}
	r, _ := setupNodeRouter()

	missingConfirmation := httptest.NewRequest(http.MethodPost, "/api/nodes/worker-a/force-drain", strings.NewReader(`{"acknowledge_risk":false,"confirm_node_name":"worker-a"}`))
	missingConfirmation.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, missingConfirmation)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "请确认风险") {
		t.Fatalf("expected confirmation rejection, got %d: %s", w.Code, w.Body.String())
	}

	confirmed := httptest.NewRequest(http.MethodPost, "/api/nodes/worker-a/force-drain", strings.NewReader(`{"acknowledge_risk":true,"confirm_node_name":"worker-a","delete_empty_dir_data":true}`))
	confirmed.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, confirmed)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"forced":true`) {
		t.Fatalf("expected force drain success, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNodeHandler_RemoveNode(t *testing.T) {
	r, _ := setupNodeRouter()
	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/web-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
