package store

import (
	"errors"
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
	"time"
)

func lifecycleProduct(t *testing.T, s *Store, owner uint, name string) model.Product {
	t.Helper()
	p := model.Product{OwnerID: owner, Name: name, Stage: "developing", Channels: []model.ProductChannel{{Platform: "app_store", PublishStatus: "draft"}}}
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestProductLifecycleIsolationHistoryAndConcurrency(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "offline")
	other := lifecycleProduct(t, s, 2, "other")
	sameOwner := lifecycleProduct(t, s, 1, "second")
	v := model.ProductVersion{ProductID: p.ID, Name: "1.0", Status: "developing"}
	if err := s.SaveProductVersion(1, &v); err != nil {
		t.Fatal(err)
	}
	duplicate := model.ProductVersion{ProductID: p.ID, Name: "1.0", Status: "planning"}
	if err := s.SaveProductVersion(1, &duplicate); !errors.Is(err, model.ErrProductVersionName) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, _, err := s.ListProductVersions(2, p.ID, model.ProductLifecycleFilter{Limit: 10}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("read isolation: %v", err)
	}
	task := model.ProductTask{ProductID: sameOwner.ID, VersionID: &v.ID, Title: "cross product", Kind: "action", Status: "todo", Priority: "normal"}
	if err := s.SaveProductTask(1, &task); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("cross product: %v", err)
	}
	task.ProductID = p.ID
	if err := s.SaveProductTask(1, &task); err != nil {
		t.Fatal(err)
	}
	stale := task
	task.Status = "done"
	if err := s.SaveProductTask(1, &task); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProductTask(1, &stale); !errors.Is(err, model.ErrProductLifecycleConflict) {
		t.Fatalf("lost update: %v", err)
	}
	record := model.ProductDistribution{Audience: "production", ProductID: p.ID, VersionID: v.ID, ChannelID: other.Channels[0].ID, PlatformVersion: "1.0", Status: "in_review"}
	if err := s.SaveProductDistribution(1, &record); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("foreign channel: %v", err)
	}
	record.ChannelID = p.Channels[0].ID
	if err := s.SaveProductDistribution(1, &record); err != nil {
		t.Fatal(err)
	}
	original := record
	record.Status = "rejected"
	record.Notes = "fix screenshots"
	if err := s.SaveProductDistribution(1, &record); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProductDistribution(1, &original); !errors.Is(err, model.ErrProductLifecycleConflict) {
		t.Fatalf("stale record: %v", err)
	}
	rows, total, err := s.ListProductDistributionEvents(1, p.ID, record.ID, model.ProductLifecycleFilter{Limit: 1, Offset: 1})
	if err != nil || total != 2 || len(rows) != 1 || rows[0].Status != "in_review" {
		t.Fatalf("history: %#v %d %v", rows, total, err)
	}
	if _, _, err := s.ListProductDistributionEvents(2, p.ID, record.ID, model.ProductLifecycleFilter{Limit: 10}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("history isolation: %v", err)
	}
	replacement := model.ProductDistribution{Audience: "production", ProductID: p.ID, VersionID: v.ID, ChannelID: record.ChannelID, PlatformVersion: "1.0", BuildIdentifier: "2", Status: "in_review"}
	if err := s.SaveProductDistribution(1, &replacement); err != nil {
		t.Fatal(err)
	}
	workbench, err := s.ProductWorkbench(1)
	if err != nil || len(workbench.Distributions) != 1 || workbench.Distributions[0].ID != replacement.ID || len(workbench.Tasks) != 0 {
		t.Fatalf("workbench: %#v %v", workbench, err)
	}
	replacement.Status = "published"
	if err := s.SaveProductDistribution(1, &replacement); err != nil {
		t.Fatal(err)
	}
	pending := model.ProductDistribution{Audience: "production", ProductID: p.ID, VersionID: v.ID, ChannelID: record.ChannelID, PlatformVersion: "1.1", Status: "in_review"}
	if err := s.SaveProductDistribution(1, &pending); err != nil {
		t.Fatal(err)
	}
	record.Notes = "historical note"
	if err := s.SaveProductDistribution(1, &record); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil || got.Channels[0].OnlineVersion != "1.0" || got.Channels[0].PendingVersion != "1.1" || got.Channels[0].PublishStatus != "in_review" {
		t.Fatalf("channel: %#v %v", got, err)
	}
	got.Channels = nil
	if err := s.SaveProduct(got, nil, nil); !errors.Is(err, model.ErrProductChannelInUse) {
		t.Fatalf("history deletion: %v", err)
	}
	got, _ = s.GetProduct(1, p.ID)
	got.Channels[0].PublishStatus = "paused"
	if err := s.SaveProduct(got, nil, nil); err != nil {
		t.Fatal(err)
	}
	workbench, err = s.ProductWorkbench(1)
	if err != nil || len(workbench.Distributions) != 0 {
		t.Fatalf("paused: %#v %v", workbench, err)
	}
}
func TestHistoricalPublishedNotesDoNotReplaceCurrentOnlineVersion(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "app")
	v := model.ProductVersion{ProductID: p.ID, Name: "1.0", Status: "completed"}
	if err := s.SaveProductVersion(1, &v); err != nil {
		t.Fatal(err)
	}
	older := model.ProductDistribution{Audience: "production", ProductID: p.ID, VersionID: v.ID, ChannelID: p.Channels[0].ID, PlatformVersion: "1.0", Status: "published"}
	newer := older
	newer.PlatformVersion = "1.1"
	for _, record := range []*model.ProductDistribution{&older, &newer} {
		if err := s.SaveProductDistribution(1, record); err != nil {
			t.Fatal(err)
		}
	}
	older.Notes = "Correct historical release notes"
	if err := s.SaveProductDistribution(1, &older); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil || got.Channels[0].OnlineVersion != "1.1" {
		t.Fatalf("editing history changed online version: %#v %v", got, err)
	}
	events, count, err := s.ListProductDistributionEvents(1, p.ID, older.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || count != 2 || events[0].BecamePublished || !events[1].BecamePublished {
		t.Fatalf("publication events: %#v %d %v", events, count, err)
	}
}

func TestProductVersionDeploymentOwnershipAndPagination(t *testing.T) {
	s := setupTestDB(t)
	p := lifecycleProduct(t, s, 1, "app")
	project := model.Project{Name: "own", OwnerID: 1}
	foreign := model.Project{Name: "foreign", OwnerID: 2}
	for _, item := range []*model.Project{&project, &foreign} {
		if err := s.CreateProject(item); err != nil {
			t.Fatal(err)
		}
	}
	app := model.Application{ProjectID: project.ID, EnvironmentID: 1, Name: "api", CreatedBy: 1}
	other := model.Application{ProjectID: foreign.ID, EnvironmentID: 2, Name: "secret", CreatedBy: 2}
	for _, item := range []*model.Application{&app, &other} {
		if err := s.CreateApplication(item); err != nil {
			t.Fatal(err)
		}
	}
	r := model.Release{ApplicationID: app.ID, Sequence: 1, Image: "image:v1", DesiredSpec: "secret", Status: "succeeded"}
	forbidden := model.Release{ApplicationID: other.ID, Sequence: 1, Image: "private:v1", DesiredSpec: "private", Status: "succeeded"}
	for _, item := range []*model.Release{&r, &forbidden} {
		if err := s.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveProduct(&p, nil, []uint{app.ID}); err != nil {
		t.Fatal(err)
	}
	v := model.ProductVersion{ProductID: p.ID, Name: "v1", Status: "testing", DeploymentIDs: []uint{forbidden.ID}}
	if err := s.SaveProductVersion(1, &v); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("foreign release: %v", err)
	}
	v.DeploymentIDs = []uint{r.ID}
	if err := s.SaveProductVersion(1, &v); err != nil {
		t.Fatal(err)
	}
	rows, total, err := s.ProductDeploymentPage(1, p.ID, model.ProductLifecycleFilter{VersionID: v.ID, Limit: 10})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ApplicationName != "api" {
		t.Fatalf("deployments: %#v %d %v", rows, total, err)
	}
	for i := 0; i < 12; i++ {
		task := model.ProductTask{ProductID: p.ID, Title: "task", Kind: "action", Status: "todo", Priority: "normal"}
		if err := s.SaveProductTask(1, &task); err != nil {
			t.Fatal(err)
		}
	}
	tasks, count, err := s.ListProductTasks(1, p.ID, model.ProductLifecycleFilter{Limit: 10, Offset: 10})
	if err != nil || count != 12 || len(tasks) != 2 {
		t.Fatalf("pagination: %d %d %v", len(tasks), count, err)
	}
	project.OwnerID = 2
	if err := s.UpdateProject(&project); err != nil {
		t.Fatal(err)
	}
	versions, _, err := s.ListProductVersions(1, p.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(versions) != 1 || len(versions[0].DeploymentIDs) != 0 {
		t.Fatalf("revocation: %#v %v", versions, err)
	}
}
func TestProductLifecycleMigrationAndLegacyAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := lifecycleProduct(t, s, 1, "legacy")
	due := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	p.NextAction = "follow review"
	p.NextDueDate = &due
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().Migrator().DropTable(&model.ProductTask{}, &model.ProductVersion{}, &model.ProductDistribution{}, &model.ProductDistributionEvent{}, &model.ProductVersionDeployment{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		s, err = New(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetProduct(1, p.ID)
	if err != nil || got.NextAction != p.NextAction || len(got.Channels) != 1 {
		t.Fatalf("migration: %#v %v", got, err)
	}
	task, err := s.ImportProductNextAction(1, p.ID)
	if err != nil || task.Title != p.NextAction || task.DueDate != "2026-11-20" {
		t.Fatalf("import: %#v %v", task, err)
	}
	if _, err := s.ImportProductNextAction(1, p.ID); !errors.Is(err, model.ErrProductLifecycleConflict) {
		t.Fatalf("duplicate import: %v", err)
	}
	got, _ = s.GetProduct(1, p.ID)
	if got.NextAction != "" || got.NextDueDate != nil {
		t.Fatal("legacy action not cleared")
	}
}
