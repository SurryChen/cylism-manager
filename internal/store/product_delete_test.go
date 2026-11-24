package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func productDeletionFixture(t *testing.T, s *Store) (model.Product, []interface{}) {
	t.Helper()
	project := model.Project{Name: "preserved", OwnerID: 1}
	if err := s.DB().Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	environment := model.Environment{ProjectID: project.ID, Name: "test", Namespace: "product-test"}
	if err := s.DB().Create(&environment).Error; err != nil {
		t.Fatal(err)
	}
	app := model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "api", CreatedBy: 1}
	if err := s.CreateApplication(&app); err != nil {
		t.Fatal(err)
	}
	release := model.Release{ApplicationID: app.ID, Sequence: 1, Image: "image:v1", DesiredSpec: "{}", Status: "succeeded"}
	platform := model.PlatformRelease{Source: "github", Image: "manager:v1", Status: "succeeded"}
	database := model.DatabaseInstance{Name: "preserved-db", Mode: "external", Status: "ready"}
	endpoint := model.ApplicationEndpoint{ApplicationID: app.ID, Domain: "example.invalid", ServicePort: 80}
	for _, record := range []interface{}{&release, &platform, &database, &endpoint} {
		if err := s.DB().Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	binding := model.DatabaseBinding{ApplicationID: app.ID, InstanceID: database.ID, TemplateID: 1, Prefix: "DB"}
	if err := s.DB().Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	p := lifecycleProduct(t, s, 1, "delete-me")
	p.PlatformLinked = true
	if err := s.SaveProduct(&p, []uint{project.ID}, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	v := model.ProductVersion{ProductID: p.ID, Name: "1.0", Status: "testing", DeploymentIDs: []uint{release.ID}, PlatformReleaseIDs: []uint{platform.ID}}
	if err := s.SaveProductVersion(1, &v); err != nil {
		t.Fatal(err)
	}
	task := model.ProductTask{ProductID: p.ID, VersionID: &v.ID, Title: "verify", Kind: "action", Status: "todo", Priority: "normal"}
	if err := s.SaveProductTask(1, &task); err != nil {
		t.Fatal(err)
	}
	d := model.ProductDistribution{ProductID: p.ID, VersionID: v.ID, ChannelID: p.Channels[0].ID, PlatformVersion: "1.0", Status: "published"}
	if err := s.SaveProductDistribution(1, &d); err != nil {
		t.Fatal(err)
	}
	return p, []interface{}{&project, &environment, &app, &release, &platform, &database, &endpoint, &binding}
}

func TestProductDeletionCleansOwnedDataAndPreservesResources(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "single"
		if all {
			name = "all"
		}
		t.Run(name, func(t *testing.T) {
			s := setupTestDB(t)
			p, resources := productDeletionFixture(t, s)
			archived := model.Product{OwnerID: 1, Name: "archived", Stage: "archived"}
			if err := s.SaveProduct(&archived, nil, nil); err != nil {
				t.Fatal(err)
			}
			other := lifecycleProduct(t, s, 2, "other")
			for _, owner := range []uint{0, 2} {
				if err := s.DeleteProduct(owner, p.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
					t.Fatalf("unauthorized deletion: %v", err)
				}
			}
			if _, err := s.ClearProducts(0); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("zero owner: %v", err)
			}
			if all {
				if count, err := s.ClearProducts(1); err != nil || count != 2 {
					t.Fatalf("clear: %d %v", count, err)
				}
			} else {
				if err := s.DeleteProduct(1, p.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := s.GetProduct(1, archived.ID); err != nil {
					t.Fatalf("unrelated product deleted: %v", err)
				}
			}
			for _, record := range []interface{}{&model.ProductTask{}, &model.ProductVersion{}, &model.ProductDistribution{}, &model.ProductDistributionEvent{}, &model.ProductVersionDeployment{}, &model.ProductVersionPlatformRelease{}, &model.ProductProject{}, &model.ProductApplication{}} {
				var count int64
				query := s.DB().Model(record)
				if _, ok := record.(*model.ProductProject); ok {
					query = query.Where("product_id = ?", p.ID)
				}
				if err := query.Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("orphan %T: %d %v", record, count, err)
				}
			}
			if _, err := s.GetProduct(1, p.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("product remains: %v", err)
			}
			if got, err := s.GetProduct(2, other.ID); err != nil || len(got.Channels) != 1 {
				t.Fatalf("other owner's data changed: %#v %v", got, err)
			}
			var channels int64
			if err := s.DB().Model(&model.ProductChannel{}).Where("product_id = ?", p.ID).Count(&channels).Error; err != nil || channels != 0 {
				t.Fatalf("channels remain: %d %v", channels, err)
			}
			for _, record := range resources {
				if err := s.DB().First(record).Error; err != nil {
					t.Fatalf("resource %T removed: %v", record, err)
				}
			}
			if err := s.DeleteProduct(1, p.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("missing product deletion: %v", err)
			}
			if _, err := s.ClearProducts(1); err != nil {
				t.Fatal(err)
			}
			if count, err := s.ClearProducts(1); err != nil || count != 0 {
				t.Fatalf("empty clear: %d %v", count, err)
			}
		})
	}
}

func TestProductDeletionRollsBackAllChildrenOnFailure(t *testing.T) {
	s := setupTestDB(t)
	p, _ := productDeletionFixture(t, s)
	if err := s.DB().Exec("CREATE TRIGGER reject_product_delete BEFORE DELETE ON products BEGIN SELECT RAISE(ABORT, 'test delete failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		if all {
			if count, err := s.ClearProducts(1); err == nil || count != 0 {
				t.Fatalf("failed clear: %d %v", count, err)
			}
		} else if err := s.DeleteProduct(1, p.ID); err == nil {
			t.Fatal("expected deletion failure")
		}
		for _, record := range []interface{}{&model.Product{}, &model.ProductTask{}, &model.ProductVersion{}, &model.ProductChannel{}, &model.ProductDistribution{}, &model.ProductDistributionEvent{}, &model.ProductVersionDeployment{}, &model.ProductVersionPlatformRelease{}, &model.ProductProject{}, &model.ProductApplication{}} {
			var count int64
			expected := int64(1)
			if _, ok := record.(*model.ProductProject); ok {
				expected = 2
			}
			if err := s.DB().Model(record).Count(&count).Error; err != nil || count != expected {
				t.Fatalf("rollback lost %T: %d %v", record, count, err)
			}
		}
	}
}
