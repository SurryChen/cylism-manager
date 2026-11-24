package cluster

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

func TestRemovalUninstallsOnlyAfterDrainStopAndNodeDeletion(t *testing.T) {
	h, st, identity, client, _ := removalHandlerFixture(t)
	identity.UninstallK3s = true
	task, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(task, k8s.DrainOptions{})
	want := []string{"drain", "stop", "delete", "stop", "uninstall"}
	if !reflect.DeepEqual(*client.events, want) {
		t.Fatalf("unsafe order: %v", *client.events)
	}
	server, _ := st.GetServer(2)
	persisted, _ := st.GetClusterDeployment(1, task.ID)
	if persisted.Status != "success" || !persisted.NodeUninstalled || server.ClusterID != nil || server.K3sRemovalTaskID != nil {
		t.Fatalf("incomplete uninstall: %#v %#v", persisted, server)
	}
}

func TestRemovalUninstallFailureCanResumeMissingNode(t *testing.T) {
	h, st, identity, client, host := removalHandlerFixture(t)
	identity.UninstallK3s = true
	host.uninstallErr = errors.New("SSH failed")
	task, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(task, k8s.DrainOptions{})
	persisted, _ := st.GetClusterDeployment(1, task.ID)
	server, _ := st.GetServer(2)
	if persisted.Status != "failed" || !persisted.NodeStopped || persisted.NodeUninstalled || server.ClusterID == nil || client.node != nil {
		t.Fatalf("lost retry context: %#v %#v", persisted, server)
	}
	plan, checkpoint, err := h.prepareRemoval(context.Background(), 1, server)
	if err != nil || !plan.NodeMissing {
		t.Fatalf("cannot resume: %#v %v", plan, err)
	}
	host.uninstallErr = nil
	retry, err := st.StartClusterNodeRemoval(1, 2, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(retry, k8s.DrainOptions{})
	server, _ = st.GetServer(2)
	if server.ClusterID != nil || server.K3sRemovalTaskID != nil {
		t.Fatal("retry did not finish cleanup")
	}
}

func TestServerCleanupRemainsAvailableWithoutOriginalClusterOrAgent(t *testing.T) {
	h, st, identity, client, host := removalHandlerFixture(t)
	removal, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(removal, k8s.DrainOptions{})
	if err := st.DB().Delete(&model.Cluster{}, 1).Error; err != nil {
		t.Fatal(err)
	}
	h.removalClient = nil
	host.uninstallErr = errors.New("host identity mismatch")
	task, err := st.StartServerK3sCleanup(2, removal.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.runServerK3sCleanup(task)
	server, _ := st.GetServer(2)
	if server.K3sRemovalTaskID == nil {
		t.Fatal("failure discarded cleanup entry")
	}
	host.uninstallErr = nil
	task, err = st.StartServerK3sCleanup(2, removal.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.runServerK3sCleanup(task)
	server, _ = st.GetServer(2)
	if server.K3sRemovalTaskID != nil || !reflect.DeepEqual(*client.events, []string{"drain", "stop", "delete", "stop", "uninstall", "uninstall"}) {
		t.Fatalf("cleanup failed: %#v %v", server, *client.events)
	}
	var audit model.AuditLog
	if err := st.DB().Where("action = ? AND resource_type = ? AND resource_id = ? AND outcome = ?", "cluster.node_cleanup", "server", server.ID, model.AuditOutcomeSucceeded).First(&audit).Error; err != nil {
		t.Fatalf("missing server cleanup completion audit: %v", err)
	}
	router := gin.New()
	router.GET("/servers/:id/k3s-cleanup", h.ServerK3sCleanupStatus)
	response := serve(router, newJSONRequest(http.MethodGet, "/servers/2/k3s-cleanup", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "success") || strings.Contains(response.Body.String(), "machine-1") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("bad status response: %d %s", response.Code, response.Body.String())
	}
}

func TestServerCleanupRejectsArbitraryHostsAndStaleConfirmation(t *testing.T) {
	h, st, identity, client, _ := removalHandlerFixture(t)
	router := gin.New()
	router.POST("/servers/:id/k3s-cleanup", h.CleanupServerK3s)
	router.POST("/clusters/:id/nodes/:serverID/uninstall", h.Uninstall)
	response := serve(router, newJSONRequest(http.MethodPost, "/clusters/1/nodes/2/uninstall", nil))
	if response.Code != 409 {
		t.Fatal("legacy worker uninstall still bypasses safety flow")
	}
	removal, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(removal, k8s.DrainOptions{})
	for _, test := range []struct {
		path string
		body any
	}{
		{"/servers/1/k3s-cleanup", gin.H{"removal_task_id": removal.ID, "confirm_node_name": "worker"}},
		{"/servers/2/k3s-cleanup", gin.H{"removal_task_id": removal.ID + 1, "confirm_node_name": "worker"}},
		{"/servers/2/k3s-cleanup", gin.H{"removal_task_id": removal.ID, "confirm_node_name": "wrong"}},
		{"/servers/2/k3s-cleanup", gin.H{}},
	} {
		if response := serve(router, newJSONRequest(http.MethodPost, test.path, test.body)); response.Code == 200 {
			t.Fatalf("unsafe cleanup accepted: %#v", test)
		}
	}
	if len(*client.events) != 4 {
		t.Fatal("invalid request executed SSH")
	}
}
