package store

import (
	"errors"
	"slices"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestTransferApplicationProjectPreservesDeploymentAndVersionLinks(t *testing.T) {
	s := setupTestDB(t)
	source := model.Project{Name: "自用", OwnerID: 1}
	target := model.Project{Name: "用户中心", OwnerID: 1}
	other := model.Project{Name: "其他用户", OwnerID: 2}
	for _, project := range []*model.Project{&source, &target, &other} {
		if err := s.CreateProject(project); err != nil {
			t.Fatal(err)
		}
	}
	env := model.Environment{ProjectID: source.ID, ClusterID: 7, Name: "production", Namespace: "personal-production"}
	if err := s.CreateEnvironment(&env); err != nil {
		t.Fatal(err)
	}
	account := model.Application{ProjectID: source.ID, EnvironmentID: env.ID, Name: "account", CreatedBy: 1}
	sibling := model.Application{ProjectID: source.ID, EnvironmentID: env.ID, Name: "other", CreatedBy: 1}
	for _, app := range []*model.Application{&account, &sibling} {
		if err := s.CreateApplication(app); err != nil {
			t.Fatal(err)
		}
	}
	release := model.Release{ApplicationID: account.ID, Sequence: 1, Version: "1.4.2", Status: model.ReleaseStatusSucceeded}
	pending := model.Release{ApplicationID: account.ID, Sequence: 2, Version: "1.4.3", Status: model.ReleaseStatusApplying}
	for _, item := range []*model.Release{&release, &pending} {
		if err := s.CreateRelease(item); err != nil {
			t.Fatal(err)
		}
	}
	sourceWorkspace, err := s.GetProjectWorkspace(1, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	version := model.ProductVersion{ProductID: sourceWorkspace.ID, Name: "1.4.2", ReleasedOn: "2026-11-20", DeploymentIDs: []uint{release.ID}}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	move := model.ApplicationProjectTransfer{ApplicationID: account.ID, FromProjectID: source.ID, ToProjectID: target.ID, OwnerID: 1}
	foreign := move
	foreign.OwnerID = 2
	if err := s.TransferApplicationProject(foreign); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign transfer: %v", err)
	}
	foreign = move
	foreign.ToProjectID = other.ID
	if err := s.TransferApplicationProject(foreign); !errors.Is(err, model.ErrApplicationTransferTarget) {
		t.Fatalf("foreign target: %v", err)
	}
	if err := s.TransferApplicationProject(move); !errors.Is(err, model.ErrApplicationTransferBusy) {
		t.Fatalf("in-progress release: %v", err)
	}
	pending.Status = model.ReleaseStatusFailed
	if err := s.UpdateRelease(&pending); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.TransferApplicationProject(move); err != nil {
			t.Fatal(err)
		}
	}
	accountAfter, err := s.GetApplication(account.ID)
	if err != nil || accountAfter.OwningProjectID() != target.ID || accountAfter.ProjectID != source.ID ||
		accountAfter.EnvironmentID != env.ID || accountAfter.Environment.Namespace != env.Namespace {
		t.Fatalf("application after transfer: %+v, %v", accountAfter, err)
	}
	runtimeApps, err := s.ListApplications(source.ID, env.ID)
	if err != nil || len(runtimeApps) != 2 {
		t.Fatalf("shared runtime workspace: %+v, %v", runtimeApps, err)
	}
	targetWorkspace, err := s.GetProjectWorkspace(1, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if apps := targetWorkspace.TransferredApplications; len(apps) != 1 || apps[0].ID != account.ID || apps[0].ActiveReleaseID != release.ID || apps[0].ActiveVersion != release.Version {
		t.Fatalf("target workspace applications: %+v", apps)
	}
	if err := s.DeleteProjectWorkspace(1, target.ID); !errors.Is(err, model.ErrProjectWorkspaceConflict) {
		t.Fatalf("target should be protected against deletion: %v", err)
	}
	filter := model.ProductLifecycleFilter{Limit: 10}
	if _, count, err := s.ProductDeploymentPage(1, sourceWorkspace.ID, filter); err != nil || count != 0 {
		t.Fatalf("source deployment options: %d, %v", count, err)
	}
	filter.VersionID = version.ID
	filter.IncludeVersionDeployments = true
	rows, count, err := s.ProductDeploymentPage(1, sourceWorkspace.ID, filter)
	if err != nil || count != 1 || rows[0].ID != release.ID {
		t.Fatalf("source version's saved links: %+v, %d, %v", rows, count, err)
	}
	versions, _, err := s.ListProductVersions(1, sourceWorkspace.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || len(versions) != 1 || !slices.Equal(versions[0].DeploymentIDs, []uint{release.ID}) {
		t.Fatalf("source versions: %+v, %v", versions, err)
	}
	version.Notes = "retained deployment"
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	targetVersion := model.ProductVersion{ProductID: targetWorkspace.ID, Name: release.Version, ReleasedOn: "2026-11-20", DeploymentIDs: []uint{release.ID}}
	if err := s.SaveProductVersion(1, &targetVersion); err != nil {
		t.Fatal(err)
	}
	move.FromProjectID, move.ToProjectID = target.ID, source.ID
	if err := s.TransferApplicationProject(move); err != nil {
		t.Fatal(err)
	}
	back, err := s.GetApplication(account.ID)
	if err != nil || back.ManagementProjectID != nil {
		t.Fatalf("application moved back: %+v, %v", back, err)
	}
}
