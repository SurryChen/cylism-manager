package store

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestGitHubVersionSourcesStayPinnedAfterUnbinding(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "multi-component")
	unit := model.ProductReleaseUnit{ProductID: product.ID, Name: "Web"}
	if err := s.SaveProductReleaseUnit(1, &unit); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&model.GitHubConnection{OwnerID: 1, AppID: 50, Slug: "test", EncryptedKey: "ciphertext"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GitHubConnection(2); !errors.Is(err, model.ErrGitHubNotConfigured) {
		t.Fatalf("read another user's credentials: %v", err)
	}
	var sources []model.ProductGitHubSource
	for id, name := range []string{"api", "web"} {
		binding := model.ProductGitHubRepository{
			ProductID: product.ID, ReleaseUnitID: unit.ID, AppID: 50, InstallationID: 2,
			RepositoryID: int64(id + 100), Component: name, Repository: "org/" + name,
		}
		if err := s.SaveProductGitHubRepository(2, &binding); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("cross-account binding: %v", err)
		}
		if err := s.SaveProductGitHubRepository(1, &binding); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, model.ProductGitHubSource{
			BindingID: binding.ID, Component: name, Repository: binding.Repository, RepositoryID: binding.RepositoryID,
			Tag: name + "/v2", SHA: strings.Repeat("a", 40),
		})
	}
	version := model.ProductVersion{ProductID: product.ID, ReleaseUnitID: unit.ID, Name: "2.0", GitHubSources: sources}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProductGitHubRepository(1, product.ID, sources[0].BindingID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGitHubConnection(1); err != nil {
		t.Fatal(err)
	}
	version.GitHubSources, version.Notes = nil, "legacy client update"
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ProductVersionGitHubSources(1, product.ID, version.ID)
	if err != nil || !slices.Equal(stored.GitHubSources, sources) {
		t.Fatalf("lost pinned sources: %v, %v", stored.GitHubSources, err)
	}
	version.GitHubSources[0].SHA = strings.Repeat("b", 40)
	if err := s.SaveProductVersion(1, &version); !errors.Is(err, model.ErrProductLifecycleConflict) {
		t.Fatalf("changed a source with no binding: %v", err)
	}
	version.GitHubSources = []model.ProductGitHubSource{}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	stored, err = s.ProductVersionGitHubSources(1, product.ID, version.ID)
	if err != nil || len(stored.GitHubSources) != 0 {
		t.Fatalf("explicit source removal failed: %v, %v", stored.GitHubSources, err)
	}
}
