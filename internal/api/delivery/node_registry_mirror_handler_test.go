package delivery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupNodeRegistryMirrorRouter() (*gin.Engine, *store.Store, *NodeRegistryMirrorHandler) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	// SQLite's :memory: database is per connection. Keep the async task and
	// request handlers on the same connection for this test fixture.
	if db, err := s.DB().DB(); err == nil {
		db.SetMaxOpenConns(1)
	}
	r := gin.New()
	h := NewNodeRegistryMirrorHandler(s, []byte("01234567890123456789012345678901"), func(_ *model.Server, _ []byte) (string, string) {
		return "success", "configured"
	})
	mirrors := r.Group("/api/node-registry-mirrors")
	{
		mirrors.GET("", h.List)
		mirrors.POST("", h.Create)
		mirrors.POST("/:id/verify", h.Verify)
		mirrors.PUT("/:id", h.Update)
		mirrors.DELETE("/:id", h.Delete)
		mirrors.POST("/:id/apply", h.Apply)
		mirrors.GET("/:id/apply-status", h.ApplyStatus)
	}
	return r, s, h
}

func TestNodeRegistryMirrorVerifyPersistsConnectionResult(t *testing.T) {
	r, s, h := setupNodeRegistryMirrorRouter()
	mirror := &model.NodeRegistryMirror{
		Name:              "docker-hub-mirror",
		Registry:          "docker.io",
		Endpoints:         `["https://mirror.example.com"]`,
		VerificationImage: "docker.io/library/busybox:1.36",
		Enabled:           true,
	}
	if err := s.CreateNodeRegistryMirror(mirror); err != nil {
		t.Fatal(err)
	}
	h.WithVerifier(func(_ context.Context, _ *model.NodeRegistryMirror, _ []byte) error { return nil })

	success := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/1/verify", nil))
	if success.Code != http.StatusOK || !strings.Contains(success.Body.String(), `"last_verify_status":"succeeded"`) {
		t.Fatalf("unexpected successful verification: %s", success.Body.String())
	}

	h.WithVerifier(func(_ context.Context, _ *model.NodeRegistryMirror, _ []byte) error {
		return errors.New("https://mirror.example.com: 认证失败 (HTTP 401)")
	})
	failure := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/1/verify", nil))
	if failure.Code != http.StatusOK || !strings.Contains(failure.Body.String(), `"last_verify_status":"failed"`) || !strings.Contains(failure.Body.String(), "认证失败") {
		t.Fatalf("unexpected failed verification: %s", failure.Body.String())
	}
}

func TestNodeRegistryMirrorRejectsVerificationImageFromAnotherRegistry(t *testing.T) {
	r, _, _ := setupNodeRegistryMirrorRouter()
	response := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors", gin.H{
		"name":               "docker-hub-mirror",
		"registry":           "docker.io",
		"endpoints":          []string{"https://mirror.example.com"},
		"verification_image": "ghcr.io/example/busybox:1.36",
	}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "验证镜像必须属于当前 Registry") {
		t.Fatalf("expected registry mismatch error: %s", response.Body.String())
	}
}

func TestNodeRegistryMirrorVerificationUsesMirrorEndpoint(t *testing.T) {
	ref, err := registryservice.MirrorVerificationReference("docker.io", "docker.io/library/busybox:1.36", "https://mirror.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name() != "mirror.example.com/library/busybox:1.36" {
		t.Fatalf("expected mirror reference, got %q", ref.Name())
	}
}

func TestNodeRegistryMirrorApplyRejectsEmptyOrNonClusterSelection(t *testing.T) {
	r, s, _ := setupNodeRegistryMirrorRouter()
	mirror := createEnabledNodeRegistryMirror(t, s)
	server := &model.Server{Name: "standalone", Host: "10.0.0.10"}
	if err := s.CreateServer(server); err != nil {
		t.Fatal(err)
	}

	empty := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/"+uintString(mirror.ID)+"/apply", gin.H{}))
	if empty.Code != http.StatusBadRequest || !strings.Contains(empty.Body.String(), "至少选择一个集群节点") {
		t.Fatalf("expected empty selection rejection: %s", empty.Body.String())
	}

	nonCluster := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/"+uintString(mirror.ID)+"/apply", gin.H{"server_ids": []uint{server.ID}}))
	if nonCluster.Code != http.StatusBadRequest || !strings.Contains(nonCluster.Body.String(), "不是可应用的集群节点") {
		t.Fatalf("expected non-cluster selection rejection: %s", nonCluster.Body.String())
	}
}

func TestNodeRegistryMirrorApplyOnlyRunsOnSelectedNodesAndPersistsProgress(t *testing.T) {
	r, s, h := setupNodeRegistryMirrorRouter()
	mirror := createEnabledNodeRegistryMirror(t, s)
	selected := createClusterServer(t, s, "worker-a", "10.0.0.11")
	other := createClusterServer(t, s, "worker-b", "10.0.0.12")
	started := make(chan uint, 1)
	allowFinish := make(chan struct{})
	h.WithApplier(func(server *model.Server, _ []byte) (string, string) {
		started <- server.ID
		<-allowFinish
		return "success", "配置已写入"
	})

	response := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/"+uintString(mirror.ID)+"/apply", gin.H{"server_ids": []uint{selected.ID}}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "应用任务已提交") || !strings.Contains(response.Body.String(), `"last_apply_status":"applying"`) {
		t.Fatalf("expected submitted apply task: %s", response.Body.String())
	}
	select {
	case got := <-started:
		if got != selected.ID {
			t.Fatalf("expected selected server %d, got %d", selected.ID, got)
		}
	case <-time.After(time.Second):
		t.Fatal("apply task did not start")
	}

	progress := serve(r, newJSONRequest(http.MethodGet, "/api/node-registry-mirrors/"+uintString(mirror.ID)+"/apply-status", nil))
	if progress.Code != http.StatusOK || !strings.Contains(progress.Body.String(), `"status":"applying"`) || strings.Contains(progress.Body.String(), `"server_id":`+uintString(other.ID)) {
		t.Fatalf("expected only selected node in progress response: %s", progress.Body.String())
	}

	duplicate := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/"+uintString(mirror.ID)+"/apply", gin.H{"server_ids": []uint{selected.ID}}))
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "已有应用任务正在执行") {
		t.Fatalf("expected duplicate apply rejection: %s", duplicate.Body.String())
	}

	close(allowFinish)
	final := waitForNodeRegistryMirrorStatus(t, s, mirror.ID, "succeeded")
	if len(final.NodeStatuses) != 1 || final.NodeStatuses[0].ServerID != selected.ID || final.NodeStatuses[0].Status != "success" {
		t.Fatalf("expected selected node success only, got %#v", final.NodeStatuses)
	}
}

func createEnabledNodeRegistryMirror(t *testing.T, s *store.Store) *model.NodeRegistryMirror {
	t.Helper()
	mirror := &model.NodeRegistryMirror{Name: "docker-hub-mirror", Registry: "docker.io", Endpoints: `["https://mirror.example.com"]`, Enabled: true}
	if err := s.CreateNodeRegistryMirror(mirror); err != nil {
		t.Fatal(err)
	}
	return mirror
}

func createClusterServer(t *testing.T, s *store.Store, name, host string) *model.Server {
	t.Helper()
	server := &model.Server{Name: name, Host: host, ClusterRole: "worker", K8sNodeName: name}
	if err := s.CreateServer(server); err != nil {
		t.Fatal(err)
	}
	return server
}

func waitForNodeRegistryMirrorStatus(t *testing.T, s *store.Store, id uint, want string) *model.NodeRegistryMirror {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mirror, err := s.GetNodeRegistryMirror(id)
		if err == nil && mirror.LastApplyStatus == want {
			return mirror
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("mirror %d did not reach %q", id, want)
	return nil
}

func uintString(value uint) string {
	return fmt.Sprintf("%d", value)
}
