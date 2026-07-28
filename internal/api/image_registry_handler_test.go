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

func setupImageRegistryRouter() (*gin.Engine, *store.Store, *ImageRegistryHandler) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewImageRegistryHandler(s, []byte("01234567890123456789012345678901"))
	registries := r.Group("/api/image-registries")
	{
		registries.GET("", h.List)
		registries.POST("", h.Create)
		registries.POST("/:id/verify", h.Verify)
		registries.PUT("/:id", h.Update)
		registries.DELETE("/:id", h.Delete)
	}
	return r, s, h
}

func TestImageRegistryHandlerCRUDKeepsCredentialSecret(t *testing.T) {
	r, s, _ := setupImageRegistryRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	create := serve(r, newJSONRequest(http.MethodPost, "/api/image-registries", gin.H{
		"name": "commerce-harbor", "endpoint": "harbor.example.com", "auth_type": "basic",
		"verification_image": "harbor.example.com/commerce/order-api:latest", "username": "robot$commerce", "credential": "registry-password", "project_ids": []uint{1},
	}))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	registryID := responseID(t, create.Body.Bytes())
	if strings.Contains(create.Body.String(), "registry-password") {
		t.Fatalf("credential must not appear in response: %s", create.Body.String())
	}
	registry, err := s.GetImageRegistry(registryID)
	if err != nil {
		t.Fatalf("get registry: %v", err)
	}
	if registry.Credential == "registry-password" || registry.Credential == "" || registry.VerificationImage != "harbor.example.com/commerce/order-api:latest" {
		t.Fatalf("credential should be encrypted at rest, got %q", registry.Credential)
	}

	list := serve(r, newJSONRequest(http.MethodGet, "/api/image-registries", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "\"credential_configured\":true") || strings.Contains(list.Body.String(), "registry-password") {
		t.Fatalf("unexpected registry list: %s", list.Body.String())
	}
	projectList := serve(r, newJSONRequest(http.MethodGet, "/api/image-registries?project_id=1", nil))
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), "commerce-harbor") {
		t.Fatalf("expected project-authorized registry: %s", projectList.Body.String())
	}

	update := serve(r, newJSONRequest(http.MethodPut, "/api/image-registries/1", gin.H{
		"name": "commerce-harbor", "endpoint": "harbor.example.com", "auth_type": "basic",
		"verification_image": "harbor.example.com/commerce/order-api:latest", "username": "robot$release", "project_ids": []uint{1},
	}))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	registry, _ = s.GetImageRegistry(registryID)
	if registry.Credential == "" || !registry.CredentialConfigured || len(registry.Projects) != 1 {
		t.Fatalf("update must retain credential and project authorization: %+v", registry)
	}
}

func TestImageRegistryHandlerRequiresMatchingVerificationImage(t *testing.T) {
	r, s, _ := setupImageRegistryRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	base := gin.H{"name": "commerce-harbor", "endpoint": "harbor.example.com", "auth_type": "anonymous", "project_ids": []uint{1}}
	missing := serve(r, newJSONRequest(http.MethodPost, "/api/image-registries", base))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "验证镜像必填") {
		t.Fatalf("expected required verification image error: %s", missing.Body.String())
	}
	base["verification_image"] = "other.example.com/commerce/order-api:latest"
	mismatch := serve(r, newJSONRequest(http.MethodPost, "/api/image-registries", base))
	if mismatch.Code != http.StatusBadRequest || !strings.Contains(mismatch.Body.String(), "必须属于当前镜像仓库地址") {
		t.Fatalf("expected endpoint mismatch error: %s", mismatch.Body.String())
	}
}

func TestImageRegistryHandlerRejectsDeleteWhileReleasesReferenceRegistry(t *testing.T) {
	r, s, _ := setupImageRegistryRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{Name: "commerce-harbor", Endpoint: "harbor.example.com", AuthType: "anonymous", Enabled: true}
	if err := s.CreateImageRegistry(registry, []uint{1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: 1, Sequence: 1, Image: "harbor.example.com/commerce/order-api:1", ImageRegistryID: &registry.ID, DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded}); err != nil {
		t.Fatal(err)
	}

	deleteResponse := serve(r, newJSONRequest(http.MethodDelete, "/api/image-registries/1", nil))
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("delete referenced registry status = %d: %s", deleteResponse.Code, deleteResponse.Body.String())
	}
}

func TestImageRegistryHandlerVerifyPersistsConnectionResult(t *testing.T) {
	r, s, h := setupImageRegistryRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{Name: "commerce-harbor", Endpoint: "harbor.example.com", VerificationImage: "harbor.example.com/commerce/order-api:latest", AuthType: "basic", Username: "robot$commerce", Credential: "encrypted", Enabled: true}
	if err := s.CreateImageRegistry(registry, []uint{1}); err != nil {
		t.Fatal(err)
	}
	h.verifyConnection = func(_ context.Context, _ *model.ImageRegistry, _ []byte) error { return nil }

	success := serve(r, newJSONRequest(http.MethodPost, "/api/image-registries/1/verify", nil))
	if success.Code != http.StatusOK || !strings.Contains(success.Body.String(), "\"last_verify_status\":\"succeeded\"") || strings.Contains(success.Body.String(), "encrypted") {
		t.Fatalf("unexpected successful verification: %s", success.Body.String())
	}

	h.verifyConnection = func(_ context.Context, _ *model.ImageRegistry, _ []byte) error {
		return errors.New("认证失败 (HTTP 401)")
	}
	failure := serve(r, newJSONRequest(http.MethodPost, "/api/image-registries/1/verify", nil))
	if failure.Code != http.StatusOK || !strings.Contains(failure.Body.String(), "\"last_verify_status\":\"failed\"") || !strings.Contains(failure.Body.String(), "认证失败") {
		t.Fatalf("unexpected failed verification: %s", failure.Body.String())
	}
}
