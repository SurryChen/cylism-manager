package applicationapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestReleasePagesScopeHistoryWithoutClusterAccess(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	empty := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/applications/%d?releases=false", app.ID), nil))
	var emptyBody struct {
		Data struct {
			Active *model.Release `json:"active_release"`
		} `json:"data"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	if empty.Code != http.StatusOK || emptyBody.Data.Active != nil {
		t.Fatalf("unreleased app: %s", empty.Body.String())
	}
	otherEnvironment := model.Environment{ProjectID: app.ProjectID, Name: "prod", Namespace: "runtime-prod"}
	if err := s.CreateEnvironment(&otherEnvironment); err != nil {
		t.Fatal(err)
	}
	other := model.Application{ProjectID: app.ProjectID, EnvironmentID: otherEnvironment.ID, Name: "other"}
	if err := s.CreateApplication(&other); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 24; i++ {
		item := model.Release{ApplicationID: app.ID, Sequence: uint(i), Status: model.ReleaseStatusSucceeded}
		if i == 24 {
			item.ApplicationID = other.ID
		}
		if err := s.CreateRelease(&item); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		fmt.Sprintf("/api/applications/%d/releases?limit=10&offset=20", app.ID),
		fmt.Sprintf("/api/workspace/releases?project_id=%d&environment_id=%d&limit=10&offset=20", app.ProjectID, app.EnvironmentID),
	} {
		response := serve(r, newJSONRequest(http.MethodGet, path, nil))
		var body struct {
			Data struct {
				Items []model.Release `json:"items"`
				Total int             `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.Data.Total != 23 || len(body.Data.Items) != 3 {
			t.Fatalf("unexpected page %s: %d %s", path, response.Code, response.Body.String())
		}
		for _, item := range body.Data.Items {
			if item.ApplicationID != app.ID {
				t.Fatal("release escaped requested scope")
			}
		}
	}
	response := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/applications/%d?releases=false", app.ID), nil))
	var body struct {
		Data struct {
			Releases []model.Release `json:"releases"`
			Active   *model.Release  `json:"active_release"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(body.Data.Releases) != 0 || body.Data.Active == nil || body.Data.Active.Sequence != 23 {
		t.Fatalf("active release must survive omitted history: %d %s", response.Code, response.Body.String())
	}
	invalid := serve(r, newJSONRequest(http.MethodGet, "/api/workspace/releases?project_id=999&environment_id=1", nil))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid project: %d", invalid.Code)
	}
	foreign := model.Project{Name: "foreign", OwnerID: 1}
	if err := s.CreateProject(&foreign); err != nil {
		t.Fatal(err)
	}
	invalid = serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/workspace/releases?project_id=%d&environment_id=%d", foreign.ID, app.EnvironmentID), nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("cross-project environment: %d %s", invalid.Code, invalid.Body.String())
	}
}
