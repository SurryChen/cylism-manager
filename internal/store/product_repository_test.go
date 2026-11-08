package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestProductOwnershipAndAtomicAssociations(t *testing.T) {
	s := setupTestDB(t)
	project := model.Project{Name: "owned", OwnerID: 7}
	other := model.Project{Name: "other", OwnerID: 8}
	if err := s.CreateProject(&project); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(&other); err != nil {
		t.Fatal(err)
	}
	p := model.Product{OwnerID: 7, Name: "offline", Stage: model.ProductStageIdea, Links: []model.ProductLink{{Kind: "repository", URL: "https://example.com/repo"}}}
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProduct(8, p.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other owner read: %v", err)
	}
	listed, err := s.ListProducts(8)
	if err != nil || len(listed) != 0 {
		t.Fatalf("other owner's list: %#v %v", listed, err)
	}
	p.Name = "must roll back"
	if err := s.SaveProduct(&p, []uint{other.ID}, nil); !errors.Is(err, ErrProductResourceAccess) {
		t.Fatalf("association error: %v", err)
	}
	got, err := s.GetProduct(7, p.ID)
	if err != nil || got.Name != "offline" || len(got.Links) != 1 {
		t.Fatalf("atomic product: %#v %v", got, err)
	}
	if err := s.SaveProduct(&p, []uint{project.ID}, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(7, p.ID)
	if err != nil || len(got.Projects) != 1 {
		t.Fatalf("linked product: %#v %v", got, err)
	}
	project.OwnerID = 8
	if err := s.UpdateProject(&project); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(7, p.ID)
	if err != nil || len(got.Projects) != 0 {
		t.Fatalf("ownership must be rechecked on read: %#v %v", got, err)
	}
}

func TestProductChannelsCannotBeMovedAcrossProducts(t *testing.T) {
	s := setupTestDB(t)
	a := model.Product{OwnerID: 1, Name: "a", Stage: "idea", Channels: []model.ProductChannel{{Platform: "web", PublishStatus: "draft"}}}
	b := model.Product{OwnerID: 1, Name: "b", Stage: "idea"}
	if err := s.SaveProduct(&a, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProduct(&b, nil, nil); err != nil {
		t.Fatal(err)
	}
	b.Channels = a.Channels
	if err := s.SaveProduct(&b, nil, nil); !errors.Is(err, ErrProductResourceAccess) {
		t.Fatalf("channel theft: %v", err)
	}
	a.Channels = nil
	a.Stage = "archived"
	if err := s.SaveProduct(&a, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(1, a.ID)
	if err != nil || len(got.Channels) != 0 || got.Stage != "archived" {
		t.Fatalf("archive: %#v %v", got, err)
	}
}

func TestProductMigrationPreservesDeploymentData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := model.Project{Name: "existing", OwnerID: 1}
	if err := s.CreateProject(&p); err != nil {
		t.Fatal(err)
	}
	app := model.Application{ProjectID: p.ID, EnvironmentID: 1, Name: "existing-api", CreatedBy: 1}
	if err := s.CreateApplication(&app); err != nil {
		t.Fatal(err)
	}
	release := model.Release{ApplicationID: app.ID, Sequence: 1, Image: "example.invalid/app:v1", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
	if err := s.DB().Create(&release).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate the previous schema in this disposable database, then upgrade twice.
	if err := s.DB().Migrator().DropTable(&model.ProductChannel{}, &model.ProductProject{}, &model.ProductApplication{}, &model.Product{}); err != nil {
		t.Fatal(err)
	}
	upgraded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := upgraded.GetProject(p.ID)
	if err != nil || got.Name != p.Name || got.OwnerID != 1 {
		t.Fatalf("project changed: %#v %v", got, err)
	}
	if !upgraded.DB().Migrator().HasTable(&model.ProductChannel{}) {
		t.Fatal("missing channel table")
	}
	var preserved model.Release
	if err := upgraded.DB().First(&preserved, release.ID).Error; err != nil || preserved.Status != release.Status || preserved.Image != release.Image {
		t.Fatalf("release changed: %#v %v", preserved, err)
	}
	if _, err := New(path); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
}

func TestProductDerivesResourcesWithoutCredentials(t *testing.T) {
	s := setupTestDB(t)
	project := model.Project{Name: "owned", OwnerID: 1}
	if err := s.CreateProject(&project); err != nil {
		t.Fatal(err)
	}
	app := model.Application{ProjectID: project.ID, EnvironmentID: 1, Name: "api", CreatedBy: 1}
	if err := s.CreateApplication(&app); err != nil {
		t.Fatal(err)
	}
	endpoint := model.ApplicationEndpoint{ApplicationID: app.ID, Domain: "example.invalid", ServicePort: 80}
	if err := s.DB().Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	database := model.DatabaseInstance{Name: "product-db", Mode: "external", Host: "private", Port: 3306, DatabaseName: "db", Username: "private-user", EncryptedPassword: "never expose", Status: "ready"}
	if err := s.DB().Create(&database).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Create(&model.DatabaseBinding{ApplicationID: app.ID, InstanceID: database.ID, TemplateID: 1, Prefix: "DB"}).Error; err != nil {
		t.Fatal(err)
	}
	p := model.Product{OwnerID: 1, Name: "product", Stage: "developing"}
	if err := s.SaveProduct(&p, []uint{project.ID}, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RelatedApplications) != 1 || len(got.RelatedApplications[0].Endpoints) != 1 || len(got.DatabaseResources) != 1 || got.DatabaseResources[0].Name != database.Name {
		t.Fatalf("derived resources: %#v", got)
	}
	// Revoke the project ownership; derived components and database bindings disappear.
	project.OwnerID = 2
	if err := s.UpdateProject(&project); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(1, p.ID)
	if err != nil || len(got.RelatedApplications) != 0 || len(got.DatabaseResources) != 0 {
		t.Fatalf("revoked resources leaked: %#v %v", got, err)
	}
	if err := s.SaveProduct(&p, nil, []uint{app.ID}); !errors.Is(err, ErrProductResourceAccess) {
		t.Fatalf("inaccessible component: %v", err)
	}
}
