package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestApplicationCapabilitiesPersistAndNormalize(t *testing.T) {
	s := setupTestDB(t)
	app := &model.Application{ProjectID: 1, EnvironmentID: 1, Name: "hysteria", WorkloadKind: "deployment", CreatedBy: 1, Capabilities: []string{"metrics", " hysteria2 ", "metrics"}}
	if err := s.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetApplication(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Capabilities) != 2 || loaded.Capabilities[0] != "hysteria2" || loaded.Capabilities[1] != "metrics" {
		t.Fatalf("unexpected capabilities: %#v", loaded.Capabilities)
	}
	if _, err := s.ReplaceApplicationCapabilities(app.ID, []string{"invalid value"}); err == nil {
		t.Fatal("expected invalid capability to be rejected")
	}
	loaded, err = s.GetApplication(app.ID)
	if err != nil || len(loaded.Capabilities) != 2 {
		t.Fatalf("invalid update must preserve capabilities: %#v err=%v", loaded.Capabilities, err)
	}
}

func TestApplicationCapabilitiesDefaultToEmptyList(t *testing.T) {
	s := setupTestDB(t)
	app := &model.Application{ProjectID: 1, EnvironmentID: 1, Name: "legacy", WorkloadKind: "deployment", CreatedBy: 1}
	if err := s.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetApplication(app.ID)
	if err != nil || loaded.Capabilities == nil || len(loaded.Capabilities) != 0 {
		t.Fatalf("expected empty capability list, got %#v err=%v", loaded.Capabilities, err)
	}
}
