package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestImportApplicationCatalogItemsCreatesDefaultsAtomically(t *testing.T) {
	st := setupTestDB(t)
	components, err := json.Marshal([]model.ApplicationCatalogComponent{
		{Name: "API", ApplicationName: "order-api", DefaultVersion: "2.3.4", Spec: json.RawMessage(`{"image":"example.com/order-api","container_port":8080}`)},
		{Name: "Web", ApplicationName: "order-web", Spec: json.RawMessage(`{"image":"example.com/order-web","container_port":80}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []model.ApplicationCatalogItem{{ID: 5, Name: "订单系统", Slug: "orders", Revision: 2, Enabled: true, EntryComponent: "order-web", ComponentsData: string(components)}}
	installations, err := st.ImportApplicationCatalogItems(1, 2, 3, items)
	if err != nil || len(installations) != 1 || len(installations[0].Components) != 2 {
		t.Fatalf("import: installations=%+v err=%v", installations, err)
	}
	if installations[0].EntryApplicationID == 0 || !installations[0].Components[1].IsEntry || installations[0].Components[1].ApplicationID != installations[0].EntryApplicationID {
		t.Fatalf("entry component not linked: %+v", installations[0])
	}
	listed, err := st.ListApplicationCatalogInstallations(1, 2)
	if err != nil || len(listed) != 1 || len(listed[0].Components) != 2 || listed[0].EntryApplicationID != installations[0].EntryApplicationID {
		t.Fatalf("listed installation: %+v err=%v", listed, err)
	}
	otherEnvironment, err := st.ListApplicationCatalogInstallations(1, 3)
	if err != nil || len(otherEnvironment) != 0 {
		t.Fatalf("installations leaked across environments: %+v err=%v", otherEnvironment, err)
	}
	apps, err := st.ListApplications(1, 2)
	if err != nil || len(apps) != 2 {
		t.Fatalf("apps: %+v err=%v", apps, err)
	}
	for _, app := range apps {
		if app.DefaultDeploymentTemplateID == nil {
			t.Fatalf("missing default template: %+v", app)
		}
		template, err := st.GetDefaultApplicationDeploymentTemplate(app.ID)
		if err != nil || template.Spec == "" || template.ApplicationID != app.ID {
			t.Fatalf("invalid default template: %+v err=%v", template, err)
		}
		if app.Name == "order-api" && template.DefaultVersion != "2.3.4" {
			t.Fatalf("catalog version was not copied to template: %+v", template)
		}
	}
	_, err = st.ImportApplicationCatalogItems(1, 2, 3, []model.ApplicationCatalogItem{
		{Name: "New", Slug: "new-app", Enabled: true, LegacySpec: `{}`},
		items[0],
	})
	if err == nil {
		t.Fatal("expected name conflict")
	}
	stored, err := st.ListApplications(1, 2)
	if err != nil || len(stored) != 2 {
		t.Fatalf("conflict must roll back all creations: %+v err=%v", stored, err)
	}
	var count int64
	if err := st.db.Model(&model.ApplicationCatalogInstallation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("conflict must roll back package records: count=%d err=%v", count, err)
	}
}

func TestDeleteUnreleasedApplicationUpdatesCatalogInstallation(t *testing.T) {
	st := setupTestDB(t)
	components, err := json.Marshal([]model.ApplicationCatalogComponent{
		{Name: "API", ApplicationName: "order-api", Spec: json.RawMessage(`{"image":"example.com/order-api"}`)},
		{Name: "Web", ApplicationName: "order-web", Spec: json.RawMessage(`{"image":"example.com/order-web"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []model.ApplicationCatalogItem{{ID: 5, Name: "订单系统", Slug: "orders", Revision: 1, Enabled: true, EntryComponent: "order-web", ComponentsData: string(components)}}
	installations, err := st.ImportApplicationCatalogItems(1, 2, 3, items)
	if err != nil {
		t.Fatal(err)
	}
	entryID := installations[0].EntryApplicationID
	if err := st.DeleteUnreleasedApplication(entryID); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListApplicationCatalogInstallations(1, 2)
	if err != nil || len(listed) != 1 || len(listed[0].Components) != 1 || listed[0].EntryApplicationID != listed[0].Components[0].ApplicationID || !listed[0].Components[0].IsEntry {
		t.Fatalf("remaining installation: %+v err=%v", listed, err)
	}
	var templates int64
	if err := st.db.Model(&model.ApplicationDeploymentTemplate{}).Where("application_id = ?", entryID).Count(&templates).Error; err != nil || templates != 0 {
		t.Fatalf("templates remain: %d err=%v", templates, err)
	}
	if err := st.DeleteUnreleasedApplication(listed[0].Components[0].ApplicationID); err != nil {
		t.Fatal(err)
	}
	listed, err = st.ListApplicationCatalogInstallations(1, 2)
	if err != nil || len(listed) != 0 {
		t.Fatalf("installation remains: %+v err=%v", listed, err)
	}
}

func TestDeleteUnreleasedApplicationRejectsRelease(t *testing.T) {
	st := setupTestDB(t)
	app := &model.Application{ProjectID: 1, EnvironmentID: 2, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 3}
	if err := st.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	if err := st.db.Create(&model.Release{ApplicationID: app.ID, Sequence: 1, Image: "example.com/order-api:1", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUnreleasedApplication(app.ID); err != model.ErrApplicationRequiresUninstall {
		t.Fatalf("unexpected delete result: %v", err)
	}
	if _, err := st.GetApplication(app.ID); err != nil {
		t.Fatalf("application deleted: %v", err)
	}
}

func TestLegacyCatalogItemRemainsImportable(t *testing.T) {
	item := model.ApplicationCatalogItem{Name: "Legacy", Slug: "legacy-api", LegacySpec: `{"image":"example.com/legacy"}`}
	components, err := item.CatalogComponents()
	if err != nil || len(components) != 1 || components[0].ApplicationName != "legacy-api" || item.CatalogEntryComponent() != "legacy-api" {
		t.Fatalf("legacy component: %+v err=%v", components, err)
	}
}

func TestApplicationCatalogMigratesLegacyTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE application_catalog_items (
		id integer PRIMARY KEY, name text NOT NULL, application_name text NOT NULL UNIQUE,
		description text, revision integer NOT NULL, spec text NOT NULL,
		created_by integer NOT NULL, created_at datetime, updated_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO application_catalog_items (id, name, application_name, revision, spec, created_by) VALUES (1, 'Legacy', 'legacy-api', 1, '{"image":"example.com/legacy"}', 1)`).Error; err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := New(path)
	if err != nil {
		t.Fatalf("migrate legacy catalog: %v", err)
	}
	item, err := st.GetApplicationCatalogItem(1)
	if err != nil {
		t.Fatal(err)
	}
	components, err := item.CatalogComponents()
	if err != nil || !item.Enabled || len(components) != 1 || components[0].ApplicationName != "legacy-api" {
		t.Fatalf("legacy item after migration: %+v err=%v", components, err)
	}
}
