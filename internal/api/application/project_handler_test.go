package applicationapi

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"testing"
)

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
