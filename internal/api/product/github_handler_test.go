package productapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestGitHubConnectionAndVersionSourceOwnership(t *testing.T) {
	repo, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	product := model.Product{OwnerID: 1, Name: "Product", Stage: "developing"}
	if err := repo.SaveProduct(&product, nil, nil); err != nil {
		t.Fatal(err)
	}
	connection := model.GitHubConnection{OwnerID: 1, AppID: 90, Slug: "team-app", EncryptedKey: "secret-ciphertext"}
	if err := repo.DB().Create(&connection).Error; err != nil {
		t.Fatal(err)
	}
	binding := model.ProductGitHubRepository{ProductID: product.ID, Component: "api", AppID: connection.AppID, RepositoryID: 12, InstallationID: 13, Repository: "team/api"}
	if err := repo.SaveProductGitHubRepository(1, &binding); err != nil {
		t.Fatal(err)
	}
	source := model.ProductGitHubSource{BindingID: binding.ID, Component: binding.Component, Repository: binding.Repository, RepositoryID: binding.RepositoryID, Tag: "v1", SHA: strings.Repeat("a", 40)}
	version := model.ProductVersion{ProductID: product.ID, Name: "1", GitHubSources: []model.ProductGitHubSource{source}}
	if err := repo.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo).WithGitHub(repo, make([]byte, 32), "https://manager.example.com")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "owner" {
			c.Set("user_id", uint(1))
		} else {
			c.Set("user_id", uint(2))
		}
	})
	router.GET("/settings/github", h.GitHubConnection)
	router.GET("/products/:id/github/repositories", h.GitHubProductRepositories)
	router.PUT("/products/:id/lifecycle/:kind/:recordID", h.SaveLifecycle)
	request := func(method, path, body, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	settings := request(http.MethodGet, "/settings/github", "", "owner")
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), connection.Slug) || strings.Contains(settings.Body.String(), connection.EncryptedKey) {
		t.Fatalf("unexpected owner settings: %d %s", settings.Code, settings.Body.String())
	}
	otherSettings := request(http.MethodGet, "/settings/github", "", "other")
	if otherSettings.Code != http.StatusOK || !strings.Contains(otherSettings.Body.String(), `"data":null`) {
		t.Fatalf("connection visible to another user: %d %s", otherSettings.Code, otherSettings.Body.String())
	}
	otherBindings := request(http.MethodGet, fmt.Sprintf("/products/%d/github/repositories", product.ID), "", "other")
	if otherBindings.Code != http.StatusNotFound {
		t.Fatalf("bindings visible to another user: %d %s", otherBindings.Code, otherBindings.Body.String())
	}
	path := fmt.Sprintf("/products/%d/lifecycle/versions/%d", product.ID, version.ID)
	payload := fmt.Sprintf(`{"name":"1","status":"completed","revision":1,"github_sources":[{"binding_id":%d,"tag":"v1","sha":"%s","repository":"forged/repo"}]}`, binding.ID, source.SHA)
	result := request(http.MethodPut, path, payload, "owner")
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), source.Repository) || strings.Contains(result.Body.String(), "forged/repo") {
		t.Fatalf("stored version source was not reused: %d %s", result.Code, result.Body.String())
	}
	stale := request(http.MethodPut, path, payload, "owner")
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale version update accepted: %d %s", stale.Code, stale.Body.String())
	}
}
