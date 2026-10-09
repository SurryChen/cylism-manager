package applicationapi

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"testing"
)

func TestApplicationHandlerProjectSlugValidationAndUniqueness(t *testing.T) {
	r, _ := setupApplicationRouter()
	for _, slug := range []string{"", "UpperCase", "bad_name", strings.Repeat("a", 41)} {
		response := serve(r, newJSONRequest(http.MethodPost, "/api/projects", gin.H{"name": "电商", "slug": slug}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("slug %q status = %d: %s", slug, response.Code, response.Body.String())
		}
	}
	first := serve(r, newJSONRequest(http.MethodPost, "/api/projects", gin.H{"name": "电商", "slug": "commerce"}))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"slug":"commerce"`) {
		t.Fatalf("create project: %d %s", first.Code, first.Body.String())
	}
	duplicate := serve(r, newJSONRequest(http.MethodPost, "/api/projects", gin.H{"name": "另一个项目", "slug": "commerce"}))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate slug status = %d: %s", duplicate.Code, duplicate.Body.String())
	}
}

func TestApplicationHandlerLegacyProjectCanSetSlug(t *testing.T) {
	r, s := setupApplicationRouter()
	project := &model.Project{Name: "旧项目", OwnerID: 1}
	if err := s.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEnvironment(&model.Environment{ProjectID: project.ID, Name: "生产环境", Namespace: "legacy-production"}); err != nil {
		t.Fatal(err)
	}
	response := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "旧项目", "slug": "legacy"}))
	if response.Code != http.StatusOK {
		t.Fatalf("set legacy slug: %d %s", response.Code, response.Body.String())
	}
	updated, err := s.GetProject(project.ID)
	if err != nil || updated.Slug != "legacy" || updated.Environments[0].Namespace != "legacy-production" {
		t.Fatalf("legacy bindings changed: %+v, %v", updated, err)
	}
	changed := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "旧项目", "slug": "other"}))
	if changed.Code != http.StatusConflict {
		t.Fatalf("changing slug with environments status = %d: %s", changed.Code, changed.Body.String())
	}
}

func TestApplicationHandler_ProjectDefaultRegistryMustBeAuthorized(t *testing.T) {
	r, s := setupApplicationRouter()
	if err := s.CreateProject(&model.Project{Name: "commerce", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{Name: "commerce-harbor", Endpoint: "harbor.example.com", AuthType: "anonymous", Enabled: true}
	if err := s.CreateImageRegistry(registry, []uint{1}); err != nil {
		t.Fatal(err)
	}

	setDefault := serve(r, newJSONRequest(http.MethodPut, "/api/projects/1", gin.H{"name": "commerce", "default_image_registry_id": registry.ID}))
	if setDefault.Code != http.StatusOK {
		t.Fatalf("set default registry status = %d: %s", setDefault.Code, setDefault.Body.String())
	}
	project, err := s.GetProject(1)
	if err != nil || project.DefaultImageRegistryID == nil || *project.DefaultImageRegistryID != registry.ID {
		t.Fatalf("expected default registry to be saved, got %+v err=%v", project, err)
	}

	other := &model.ImageRegistry{Name: "other-harbor", Endpoint: "other.example.com", AuthType: "anonymous", Enabled: true}
	if err := s.CreateImageRegistry(other, []uint{1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(&model.Project{Name: "payments", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	invalid := serve(r, newJSONRequest(http.MethodPut, "/api/projects/2", gin.H{"name": "payments", "default_image_registry_id": registry.ID}))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("default registry without project authorization status = %d: %s", invalid.Code, invalid.Body.String())
	}
}
