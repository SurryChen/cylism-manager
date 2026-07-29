package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupNodeRegistryMirrorRouter() (*gin.Engine, *store.Store, *NodeRegistryMirrorHandler) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewNodeRegistryMirrorHandler(s, []byte("01234567890123456789012345678901"))
	mirrors := r.Group("/api/node-registry-mirrors")
	{
		mirrors.GET("", h.List)
		mirrors.POST("", h.Create)
		mirrors.POST("/:id/verify", h.Verify)
		mirrors.PUT("/:id", h.Update)
		mirrors.DELETE("/:id", h.Delete)
		mirrors.POST("/:id/apply", h.Apply)
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
	h.verifyConnection = func(_ context.Context, _ *model.NodeRegistryMirror, _ []byte) error { return nil }

	success := serve(r, newJSONRequest(http.MethodPost, "/api/node-registry-mirrors/1/verify", nil))
	if success.Code != http.StatusOK || !strings.Contains(success.Body.String(), `"last_verify_status":"succeeded"`) {
		t.Fatalf("unexpected successful verification: %s", success.Body.String())
	}

	h.verifyConnection = func(_ context.Context, _ *model.NodeRegistryMirror, _ []byte) error {
		return errors.New("https://mirror.example.com: 认证失败 (HTTP 401)")
	}
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
	ref, err := nodeRegistryMirrorVerificationReference(&model.NodeRegistryMirror{
		Registry:          "docker.io",
		VerificationImage: "docker.io/library/busybox:1.36",
	}, "https://mirror.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name() != "mirror.example.com/library/busybox:1.36" {
		t.Fatalf("expected mirror reference, got %q", ref.Name())
	}
}
