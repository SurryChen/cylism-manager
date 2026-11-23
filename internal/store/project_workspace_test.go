package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestProjectWorkspaceMigrationPreservesLifecycleAndDeploymentIDs(t *testing.T) {
	s := setupTestDB(t)
	if err := s.DB().Where("key = ?", "migration.project_workspaces.v1").Delete(&model.SystemConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	project := model.Project{Name: "existing", Slug: "existing", Description: "Existing deployment description", OwnerID: 7}
	other := model.Project{Name: "other", OwnerID: 8}
	for _, p := range []*model.Project{&project, &other} {
		if err := s.DB().Create(p).Error; err != nil {
			t.Fatal(err)
		}
	}
	environment := model.Environment{ProjectID: project.ID, Name: "production", Namespace: "existing-prod"}
	product := model.Product{OwnerID: 7, Name: "Existing product", Stage: "developing"}
	if err := s.DB().Create(&environment).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Create(&model.ProductProject{ProductID: product.ID, ProjectID: project.ID}).Error; err != nil {
		t.Fatal(err)
	}
	task := model.ProductTask{ProductID: product.ID, Title: "preserved", Kind: "requirement", Status: "todo", Priority: "normal"}
	version := model.ProductVersion{ProductID: product.ID, Name: "1.0", Status: "testing"}
	for _, row := range []interface{}{&task, &version} {
		if err := s.DB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.backfillProjectWorkspaces(); err != nil {
		t.Fatal(err)
	}
	if err := s.backfillProjectWorkspaces(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProjectWorkspace(7, project.ID)
	if err != nil || got.ID != product.ID || got.ProjectSlug != "existing" {
		t.Fatalf("workspace: %#v %v", got, err)
	}
	var preserved model.Project
	if err := s.DB().First(&preserved, project.ID).Error; err != nil || preserved.Description != project.Description {
		t.Fatalf("project description changed: %#v %v", preserved, err)
	}
	var env model.Environment
	if err := s.DB().First(&env, environment.ID).Error; err != nil || env.ProjectID != project.ID || env.Namespace != environment.Namespace || env.Kind != "unclassified" {
		t.Fatalf("environment changed: %#v %v", env, err)
	}
	var count int64
	if err := s.DB().Model(&model.Product{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("idempotence: %d %v", count, err)
	}
	for _, row := range []interface{}{&task, &version} {
		if err := s.DB().First(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetProjectWorkspace(8, project.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross owner: %v", err)
	}
}

func TestProjectWorkspaceCreationAndSafeDeletion(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "Idea without a cluster")
	if p.ProjectID == nil || p.ProjectSlug == "" {
		t.Fatalf("missing project: %#v", p)
	}
	got, err := s.GetProjectWorkspace(1, *p.ProjectID)
	if err != nil || got.ID != p.ID {
		t.Fatalf("workspace: %#v %v", got, err)
	}
	if err := s.DeleteProjectWorkspace(2, *p.ProjectID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("delete isolation: %v", err)
	}
	env := model.Environment{ProjectID: *p.ProjectID, Name: "test", Namespace: "idea-test"}
	if err := s.DB().Create(&env).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProjectWorkspace(1, *p.ProjectID); !errors.Is(err, model.ErrProjectWorkspaceConflict) {
		t.Fatalf("resource protection: %v", err)
	}
	if _, err := s.GetProduct(1, p.ID); err != nil {
		t.Fatalf("failed delete changed metadata: %v", err)
	}
	if err := s.DB().Delete(&env).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProjectWorkspace(1, *p.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProduct(1, p.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("orphan metadata: %v", err)
	}
	// Projects created by existing deployment APIs also have lifecycle metadata.
	project := model.Project{OwnerID: 1, Name: "deployed later", Slug: "later"}
	if err := s.CreateProject(&project); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProjectWorkspace(1, project.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTestDistributionDoesNotOverwriteProductionSummary(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "audiences")
	v := model.ProductVersion{ProductID: p.ID, Name: "1", Status: "testing"}
	if err := s.SaveProductVersion(1, &v); err != nil {
		t.Fatal(err)
	}
	formal := model.ProductDistribution{Audience: "production", ProductID: p.ID, VersionID: v.ID, ChannelID: p.Channels[0].ID, PlatformVersion: "1.0", Status: "published"}
	test := model.ProductDistribution{Audience: "testing", ProductID: p.ID, VersionID: v.ID, ChannelID: p.Channels[0].ID, PlatformVersion: "2.0-beta", Status: "published"}
	for _, d := range []*model.ProductDistribution{&formal, &test} {
		if err := s.SaveProductDistribution(1, d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil || got.Channels[0].OnlineVersion != "1.0" || got.Channels[0].PendingVersion != "" {
		t.Fatalf("test replaced formal: %#v %v", got, err)
	}
	events, _, err := s.ListProductDistributionEvents(1, p.ID, test.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(events) != 1 || events[0].Audience != "testing" {
		t.Fatalf("audience snapshot: %#v %v", events, err)
	}
	// Reclassification of a historical record must not leave a stale online summary.
	formal.Audience = "testing"
	if err := s.SaveProductDistribution(1, &formal); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(1, p.ID)
	if err != nil || got.Channels[0].OnlineVersion != "" || got.Channels[0].PublishStatus != "draft" {
		t.Fatalf("stale formal summary: %#v %v", got, err)
	}
}
