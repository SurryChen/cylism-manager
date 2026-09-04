package delivery

import (
	"net/http"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
