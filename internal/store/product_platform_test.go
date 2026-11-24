package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestProductPlatformMigrationPreservesExistingProducts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := model.Product{OwnerID: 1, Name: "existing", Stage: "operating"}
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Migrator().DropTable(&model.ProductVersionPlatformRelease{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Migrator().DropColumn(&model.Product{}, "platform_linked"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		upgraded, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := upgraded.GetProduct(1, p.ID)
		if err != nil || got.Name != p.Name || got.Stage != p.Stage || got.PlatformLinked {
			t.Fatalf("upgrade changed existing product: %#v %v", got, err)
		}
		if !upgraded.DB().Migrator().HasTable(&model.ProductVersionPlatformRelease{}) {
			t.Fatal("missing platform release association table")
		}
	}
}

func TestProductPlatformAssociationAndVersionReleases(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "Cylism Manager")
	project := model.Project{Name: "API", OwnerID: 1}
	if err := s.CreateProject(&project); err != nil {
		t.Fatal(err)
	}
	app := model.Application{Name: "api", ProjectID: project.ID, EnvironmentID: 1, CreatedBy: 1}
	if err := s.CreateApplication(&app); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	release := model.Release{ApplicationID: app.ID, Sequence: 1, Version: "1.0", Image: "image:v1", DesiredSpec: "private-spec", Status: "succeeded", CreatedAt: now}
	update := model.PlatformRelease{Source: "github", Image: "ghcr.io/example/manager:1.1", Status: "succeeded", Detail: "private-log", CreatedAt: now.Add(time.Minute)}
	if err := s.DB().Create(&release).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePlatformRelease(&update); err != nil {
		t.Fatal(err)
	}
	if release.ID != update.ID {
		t.Fatal("fixture must exercise colliding application and platform IDs")
	}
	version := model.ProductVersion{ProductID: p.ID, Name: "1.1", Status: "testing", PlatformReleaseIDs: []uint{update.ID}}
	if err := s.SaveProductVersion(1, &version); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("unlinked platform release: %v", err)
	}
	rows, total, err := s.ProductDeploymentPage(1, p.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("unlinked platform visible: %#v %d %v", rows, total, err)
	}
	p.PlatformLinked = true
	if err := s.SaveProduct(&p, nil, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil || !got.PlatformLinked || len(got.Applications) != 1 {
		t.Fatalf("platform association not persisted: %#v %v", got, err)
	}
	version.DeploymentIDs = []uint{release.ID}
	version.PlatformReleaseIDs = []uint{update.ID, update.ID}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	versions, _, err := s.ListProductVersions(1, p.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(versions) != 1 || len(versions[0].DeploymentIDs) != 1 || len(versions[0].PlatformReleaseIDs) != 1 {
		t.Fatalf("distinct release associations: %#v %v", versions, err)
	}
	for offset, source := range []string{"platform", "application"} {
		rows, total, err := s.ProductDeploymentPage(1, p.ID, model.ProductLifecycleFilter{VersionID: version.ID, Limit: 1, Offset: offset})
		if err != nil || total != 2 || len(rows) != 1 || rows[0].Source != source {
			t.Fatalf("mixed pagination: %#v %d %v", rows, total, err)
		}
		data, err := json.Marshal(rows)
		if err != nil || strings.Contains(string(data), "private-") {
			t.Fatalf("release details leaked: %s %v", data, err)
		}
	}
	rows, total, err = s.ProductDeploymentPage(1, p.ID, model.ProductLifecycleFilter{Keyword: "MANAGER:1.1", Limit: 10})
	if err != nil || total != 1 || rows[0].Source != "platform" {
		t.Fatalf("platform keyword search: %#v %d %v", rows, total, err)
	}
	if _, _, err := s.ProductDeploymentPage(2, p.ID, model.ProductLifecycleFilter{Limit: 10}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign product: %v", err)
	}
	if err := s.SaveProductVersion(2, &version); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign version update: %v", err)
	}
	version.PlatformReleaseIDs = []uint{update.ID + 100}
	if err := s.SaveProductVersion(1, &version); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("unknown platform release: %v", err)
	}
	// Unlinking hides platform history without deleting its previous associations.
	p.PlatformLinked = false
	if err := s.SaveProduct(&p, nil, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	versions, _, err = s.ListProductVersions(1, p.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(versions[0].PlatformReleaseIDs) != 0 || len(versions[0].DeploymentIDs) != 1 {
		t.Fatalf("unlinked visibility: %#v %v", versions, err)
	}
	rows, total, err = s.ProductDeploymentPage(1, p.ID, model.ProductLifecycleFilter{VersionID: version.ID, Limit: 10})
	if err != nil || total != 1 || rows[0].Source != "application" {
		t.Fatalf("unlink history: %#v %d %v", rows, total, err)
	}
	p.PlatformLinked = true
	if err := s.SaveProduct(&p, nil, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	versions, _, err = s.ListProductVersions(1, p.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(versions[0].PlatformReleaseIDs) != 1 {
		t.Fatalf("relink lost history: %#v %v", versions, err)
	}
}
