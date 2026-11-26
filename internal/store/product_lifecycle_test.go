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

func TestProductReleaseUnitChannelAndVersionHistory(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "multi-platform")
	android := model.ProductReleaseUnit{ProductID: product.ID, Name: "Android"}
	desktop := model.ProductReleaseUnit{ProductID: product.ID, Name: "Desktop"}
	for _, unit := range []*model.ProductReleaseUnit{&android, &desktop} {
		if err := s.SaveProductReleaseUnit(1, unit); err != nil {
			t.Fatal(err)
		}
	}
	product.Channels[0].ReleaseUnitID = &android.ID
	if err := s.SaveProduct(&product, nil, nil); err != nil {
		t.Fatal(err)
	}
	channel := product.Channels[0]
	distribution := model.ProductDistribution{ProductID: product.ID, ChannelID: channel.ID, PlatformVersion: "2.8.1", Status: "in_review", Audience: "production"}
	if err := s.SaveProductDistribution(1, &distribution); err != nil {
		t.Fatal(err)
	}
	androidVersion := model.ProductVersion{ProductID: product.ID, ReleaseUnitID: android.ID, Name: "2.8.1", ReleasedOn: "2026-11-24"}
	desktopVersion := model.ProductVersion{ProductID: product.ID, ReleaseUnitID: desktop.ID, Name: "2.8.1", ReleasedOn: "2026-11-24"}
	for _, version := range []*model.ProductVersion{&androidVersion, &desktopVersion} {
		if err := s.SaveProductVersion(1, version); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteProductReleaseUnit(1, product.ID, android.ID); !errors.Is(err, model.ErrProductReleaseUnitInUse) {
		t.Fatalf("delete linked unit: %v", err)
	}
	distribution.VersionID = desktopVersion.ID
	if err := s.SaveProductDistribution(1, &distribution); !errors.Is(err, model.ErrProductChannelReleaseUnit) {
		t.Fatalf("linked to incompatible version: %v", err)
	}
	distribution.VersionID = androidVersion.ID
	distribution.PlatformVersion = ""
	distribution.Status = "published"
	if err := s.SaveProductDistribution(1, &distribution); err != nil || distribution.PlatformVersion != androidVersion.Name {
		t.Fatalf("attach released version: %#v, %v", distribution, err)
	}
	rows, total, err := s.ListProductDistributions(1, product.ID, model.ProductLifecycleFilter{ReleaseUnitID: &android.ID, Limit: 10})
	if err != nil || total != 1 || rows[0].VersionID != androidVersion.ID {
		t.Fatalf("filter by release unit: %#v %d %v", rows, total, err)
	}
	events, total, err := s.ListProductDistributionEvents(1, product.ID, distribution.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || total != 2 || events[1].Status != "in_review" {
		t.Fatalf("issuance history: %#v %d %v", events, total, err)
	}
}

func TestProductVersionTaskMigrationPreservesLegacyAssociation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "product-version-tasks.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := lifecycleProduct(t, s, 1, "legacy")
	version := model.ProductVersion{ProductID: p.ID, Name: "1.0", Status: "completed"}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	task := model.ProductTask{ProductID: p.ID, VersionID: &version.ID, Title: "Legacy", Kind: "action", Status: "done", Priority: "normal"}
	if err := s.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Where("key = ?", "migration.product_version_tasks.v1").Delete(&model.SystemConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Exec("CREATE UNIQUE INDEX idx_product_version_name ON product_versions(product_id, name)").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		upgraded, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		if upgraded.db.Migrator().HasIndex(&model.ProductVersion{}, "idx_product_version_name") {
			t.Fatal("legacy uniqueness index was retained")
		}
		rows, total, err := upgraded.ListProductTasks(1, p.ID, model.ProductLifecycleFilter{VersionID: version.ID, Limit: 10})
		if err != nil || total != 1 || rows[0].ID != task.ID || len(rows[0].VersionIDs) != 1 || rows[0].VersionIDs[0] != version.ID {
			t.Fatalf("migrated task: %#v %d %v", rows, total, err)
		}
	}
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

func TestProductDistributionKeepsEnvironmentAndDeploymentAssociations(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "environment-linked")
	project := model.Project{OwnerID: 1, Name: "distribution deploy project", Slug: "distribution-deploy"}
	if err := s.DB().Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	testingEnvironment := model.Environment{ProjectID: project.ID, Name: "testing", Kind: "testing", Namespace: "distribution-testing"}
	productionEnvironment := model.Environment{ProjectID: project.ID, Name: "production", Kind: "production", Namespace: "distribution-production"}
	for _, environment := range []*model.Environment{&testingEnvironment, &productionEnvironment} {
		if err := s.DB().Create(environment).Error; err != nil {
			t.Fatal(err)
		}
	}
	application := model.Application{ProjectID: project.ID, EnvironmentID: testingEnvironment.ID, Name: "api", CreatedBy: 1}
	if err := s.CreateApplication(&application); err != nil {
		t.Fatal(err)
	}
	productionApplication := model.Application{ProjectID: project.ID, EnvironmentID: productionEnvironment.ID, Name: "api", CreatedBy: 1}
	if err := s.CreateApplication(&productionApplication); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProduct(&product, []uint{project.ID}, []uint{application.ID, productionApplication.ID}); err != nil {
		t.Fatal(err)
	}
	testingRelease := model.Release{ApplicationID: application.ID, Sequence: 1, Image: "api:1.0", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
	productionRelease := model.Release{ApplicationID: productionApplication.ID, Sequence: 1, Image: "api:1.0", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
	for _, release := range []*model.Release{&testingRelease, &productionRelease} {
		if err := s.DB().Create(release).Error; err != nil {
			t.Fatal(err)
		}
	}
	version := model.ProductVersion{ProductID: product.ID, Name: "1.0.0-rc.1"}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	distribution := model.ProductDistribution{
		ProductID: product.ID, VersionID: version.ID, ChannelID: product.Channels[0].ID,
		EnvironmentID: testingEnvironment.ID, DeploymentIDs: []uint{testingRelease.ID},
		Audience: "testing", PlatformVersion: "1.0.0-rc.1", Status: "preparing",
	}
	if err := s.SaveProductDistribution(1, &distribution); err != nil {
		t.Fatal(err)
	}
	rows, total, err := s.ListProductDistributions(1, product.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].EnvironmentName != "testing" || len(rows[0].DeploymentIDs) != 1 || rows[0].DeploymentIDs[0] != testingRelease.ID {
		t.Fatalf("distribution environment associations: %#v %d %v", rows, total, err)
	}
	deployments, count, err := s.ProductDeploymentPage(1, product.ID, model.ProductLifecycleFilter{EnvironmentID: &testingEnvironment.ID, Limit: 10})
	if err != nil || count != 1 || len(deployments) != 1 || deployments[0].ID != testingRelease.ID || deployments[0].EnvironmentKind != "testing" {
		t.Fatalf("filtered environment deployments: %#v %d %v", deployments, count, err)
	}
	if _, _, err := s.ProductDeploymentPage(1, product.ID, model.ProductLifecycleFilter{EnvironmentID: func() *uint { id := uint(999); return &id }(), Limit: 10}); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("foreign environment deployment list: %v", err)
	}
	events, count, err := s.ListProductDistributionEvents(1, product.ID, distribution.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || count != 1 || events[0].EnvironmentID != testingEnvironment.ID || events[0].EnvironmentName != testingEnvironment.Name || len(events[0].DeploymentIDs) != 1 || events[0].DeploymentIDs[0] != testingRelease.ID {
		t.Fatalf("distribution event snapshot: %#v %d %v", events, count, err)
	}
	distribution.Notes = "legacy client update"
	distribution.KeepEnvironment, distribution.KeepDeployments = true, true
	if err := s.SaveProductDistribution(1, &distribution); err != nil {
		t.Fatalf("legacy update removed associations: %v", err)
	}
	if distribution.EnvironmentID != testingEnvironment.ID || len(distribution.DeploymentIDs) != 1 || distribution.DeploymentIDs[0] != testingRelease.ID {
		t.Fatalf("legacy update changed deployment associations: %#v", distribution)
	}
	distribution.KeepEnvironment, distribution.KeepDeployments = false, false
	distribution.EnvironmentID = testingEnvironment.ID
	distribution.DeploymentIDs = []uint{productionRelease.ID}
	if err := s.SaveProductDistribution(1, &distribution); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("deployment from another environment was accepted: %v", err)
	}
	distribution.EnvironmentID = 0
	distribution.DeploymentIDs = []uint{testingRelease.ID}
	if err := s.SaveProductDistribution(1, &distribution); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("deployment without environment was accepted: %v", err)
	}
	foreignProject := model.Project{OwnerID: 1, Name: "unlinked deploy project"}
	if err := s.DB().Create(&foreignProject).Error; err != nil {
		t.Fatal(err)
	}
	foreignEnvironment := model.Environment{ProjectID: foreignProject.ID, Name: "other", Namespace: "unlinked-other"}
	if err := s.DB().Create(&foreignEnvironment).Error; err != nil {
		t.Fatal(err)
	}
	distribution.EnvironmentID = foreignEnvironment.ID
	distribution.DeploymentIDs = []uint{}
	if err := s.SaveProductDistribution(1, &distribution); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("unlinked environment was accepted: %v", err)
	}
	var saved model.ProductDistribution
	if err := s.DB().First(&saved, distribution.ID).Error; err != nil || saved.EnvironmentID != testingEnvironment.ID || len(saved.DeploymentIDs) != 1 || saved.DeploymentIDs[0] != testingRelease.ID {
		t.Fatalf("failed updates changed stored associations: %#v %v", saved, err)
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
