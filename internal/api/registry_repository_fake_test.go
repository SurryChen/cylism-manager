package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type imageRegistryRepositoryFake struct {
	created    *model.ImageRegistry
	projectIDs []uint
}

var _ repository.ImageRegistryRepository = (*imageRegistryRepositoryFake)(nil)

func (f *imageRegistryRepositoryFake) CreateImageRegistry(registry *model.ImageRegistry, projectIDs []uint) error {
	registry.ID = 1
	copy := *registry
	f.created, f.projectIDs = &copy, append([]uint(nil), projectIDs...)
	return nil
}
func (f *imageRegistryRepositoryFake) ListImageRegistries(uint) ([]model.ImageRegistry, error) {
	return nil, nil
}
func (f *imageRegistryRepositoryFake) GetImageRegistry(id uint) (*model.ImageRegistry, error) {
	if f.created != nil && id == f.created.ID {
		copy := *f.created
		return &copy, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *imageRegistryRepositoryFake) GetImageRegistryByEndpoint(endpoint string) (*model.ImageRegistry, error) {
	if f.created != nil && endpoint == f.created.Endpoint {
		copy := *f.created
		return &copy, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *imageRegistryRepositoryFake) GetImageRegistryForProject(id, projectID uint) (*model.ImageRegistry, error) {
	return f.GetImageRegistry(id)
}
func (f *imageRegistryRepositoryFake) UpdateImageRegistry(registry *model.ImageRegistry, projectIDs []uint) error {
	copy := *registry
	f.created, f.projectIDs = &copy, append([]uint(nil), projectIDs...)
	return nil
}
func (f *imageRegistryRepositoryFake) UpdateImageRegistryVerification(id uint, status, detail string, verifiedAt time.Time) error {
	if f.created == nil || id != f.created.ID {
		return gorm.ErrRecordNotFound
	}
	f.created.LastVerifiedAt, f.created.LastVerifyStatus, f.created.LastVerifyError = &verifiedAt, status, detail
	return nil
}
func (f *imageRegistryRepositoryFake) CountImageRegistryReleases(uint) (int64, error) { return 0, nil }
func (f *imageRegistryRepositoryFake) DeleteImageRegistry(uint) error                 { return nil }

func TestImageRegistryHandlerCreatesThroughRepositoryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &imageRegistryRepositoryFake{}
	handler := NewImageRegistryHandler(repo, []byte("01234567890123456789012345678901"))
	router := gin.New()
	router.POST("/api/image-registries", handler.Create)

	response := serve(router, newJSONRequest(http.MethodPost, "/api/image-registries", gin.H{
		"name": "platform", "endpoint": "registry.example.com", "auth_type": "basic", "username": "robot",
		"credential": "secret-password", "project_ids": []uint{3},
		"verification_image": "registry.example.com/platform/app:1",
	}))
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if repo.created == nil || repo.created.Credential == "" || repo.created.Credential == "secret-password" || len(repo.projectIDs) != 1 {
		t.Fatalf("handler did not persist encrypted Registry configuration through repository: %#v", repo)
	}
	if responseBody := response.Body.String(); strings.Contains(responseBody, "secret-password") {
		t.Fatalf("credential leaked in response: %s", responseBody)
	}
}

type chartRepositoryFake struct {
	created *model.ChartRepository
}

var _ repository.ChartRepositoryStore = (*chartRepositoryFake)(nil)

func (f *chartRepositoryFake) CreateChartRepository(item *model.ChartRepository) error {
	item.ID = 1
	copy := *item
	f.created = &copy
	return nil
}
func (f *chartRepositoryFake) ListChartRepositories() ([]model.ChartRepository, error) {
	return nil, nil
}
func (f *chartRepositoryFake) GetChartRepository(id uint) (*model.ChartRepository, error) {
	if f.created != nil && id == f.created.ID {
		copy := *f.created
		return &copy, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *chartRepositoryFake) UpdateChartRepository(item *model.ChartRepository) error {
	copy := *item
	f.created = &copy
	return nil
}
func (f *chartRepositoryFake) DeleteChartRepository(uint) error { return nil }
func (f *chartRepositoryFake) GetVerifiedCertManagerChartRepository() (*model.ChartRepository, error) {
	return nil, gorm.ErrRecordNotFound
}

func TestChartRepositoryHandlerCreatesThroughRepositoryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &chartRepositoryFake{}
	handler := NewChartRepositoryHandler(repo)
	router := gin.New()
	router.POST("/api/chart-repositories", handler.Create)

	response := serve(router, newJSONRequest(http.MethodPost, "/api/chart-repositories", gin.H{
		"name": "jetstack", "endpoint": "https://charts.jetstack.io", "chart_name": "cert-manager", "chart_version": "v1.16.0",
	}))
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if repo.created == nil || repo.created.ID != 1 || repo.created.Endpoint != "https://charts.jetstack.io" {
		t.Fatalf("handler did not persist Chart repository through repository: %#v", repo.created)
	}
}
