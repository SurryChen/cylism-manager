package store

import (
	"path/filepath"
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

func TestApplicationManagedFileUsesOptimisticVersion(t *testing.T) {
	s := setupTestDB(t)
	file := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "secret", ResourceName: "hysteria-secret", Key: "config.yaml", MountPath: "/etc/hysteria/config.yaml", Format: "text", CreatedBy: 1}
	if err := s.CreateApplicationManagedFile(file); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetApplicationManagedFile(7, file.ID)
	if err != nil || loaded.Version != 1 {
		t.Fatalf("file=%#v err=%v", loaded, err)
	}
	if _, err := s.AdvanceApplicationManagedFileVersion(7, file.ID, 2); err == nil {
		t.Fatal("expected stale version rejection")
	}
	version, err := s.AdvanceApplicationManagedFileVersion(7, file.ID, 1)
	if err != nil || version != 2 {
		t.Fatalf("version=%d err=%v", version, err)
	}
}

func TestDisableApplicationManagedFilesNotInDisablesRemovedBindings(t *testing.T) {
	s := setupTestDB(t)
	keep := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "configmap", ResourceName: "app-config", Key: "keep.yaml", MountPath: "/etc/app/keep.yaml", CreatedBy: 1}
	remove := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "secret", ResourceName: "app-secret", Key: "remove.yaml", MountPath: "/etc/app/remove.yaml", CreatedBy: 1}
	if err := s.CreateApplicationManagedFile(keep); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplicationManagedFile(remove); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]struct{}{"configmap\x00app-config\x00keep.yaml": {}}
	if err := s.DisableApplicationManagedFilesNotIn(7, bindings); err != nil {
		t.Fatal(err)
	}
	files, err := s.ListApplicationManagedFiles(7)
	if err != nil || len(files) != 1 || files[0].ID != keep.ID {
		t.Fatalf("enabled files=%#v err=%v", files, err)
	}
}

func TestApplicationDeploymentTemplateMigrationRemovesLegacySingleTemplateIndex(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "store.db")
	st, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	app := &model.Application{ProjectID: 1, EnvironmentID: 1, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}
	if err := st.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	legacy := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "legacy", Enabled: true, Spec: "{}", Revision: 1}
	if err := st.DB().Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(legacy).Updates(map[string]interface{}{"name": "", "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Exec("CREATE UNIQUE INDEX idx_application_deployment_templates_application_id ON application_deployment_templates(application_id)").Error; err != nil {
		t.Fatal(err)
	}

	migrated, err := New(dsn)
	if err != nil {
		t.Fatalf("migrate legacy template database: %v", err)
	}
	templates, err := migrated.ListApplicationDeploymentTemplates(app.ID)
	if err != nil || len(templates) != 1 || templates[0].Name != "默认模板" || !templates[0].Enabled {
		t.Fatalf("legacy template was not backfilled: %#v err=%v", templates, err)
	}
	if err := migrated.CreateApplicationDeploymentTemplate(&model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "灰度模板", Enabled: true, Spec: "{}"}, false); err != nil {
		t.Fatalf("legacy single-template index should have been removed: %v", err)
	}
}
