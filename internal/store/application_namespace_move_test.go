package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAdoptApplicationNamespaceAfterProjectTransfer(t *testing.T) {
	s := setupTestDB(t)
	source := model.Project{Name: "自用", OwnerID: 1}
	target := model.Project{Name: "用户中心", OwnerID: 1}
	for _, project := range []*model.Project{&source, &target} {
		if err := s.CreateProject(project); err != nil {
			t.Fatal(err)
		}
	}
	origin := model.Environment{ProjectID: source.ID, ClusterID: 7, Name: "app", Namespace: "project-self-app"}
	destination := model.Environment{ProjectID: target.ID, ClusterID: 7, Name: "testing", Namespace: "project-account-testing"}
	for _, env := range []*model.Environment{&origin, &destination} {
		if err := s.CreateEnvironment(env); err != nil {
			t.Fatal(err)
		}
	}
	app := model.Application{Name: "account", ProjectID: source.ID, EnvironmentID: origin.ID}
	if err := s.CreateApplication(&app); err != nil {
		t.Fatal(err)
	}
	release := model.Release{ApplicationID: app.ID, Sequence: 1, Version: "v1", Status: model.ReleaseStatusSucceeded}
	if err := s.CreateRelease(&release); err != nil {
		t.Fatal(err)
	}
	move := model.ApplicationNamespaceMove{ApplicationID: app.ID, SourceEnvironmentID: origin.ID, TargetEnvironmentID: destination.ID, ReleaseID: release.ID, OwnerID: 1}
	if err := s.CheckApplicationNamespaceMove(move); err == nil {
		t.Fatal("accepted target before transferring to its project")
	}
	if err := s.TransferApplicationProject(model.ApplicationProjectTransfer{ApplicationID: app.ID, FromProjectID: source.ID, ToProjectID: target.ID, OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	domain := model.ManagedDomain{Hostname: "account.example.com", Namespace: origin.Namespace, EnvironmentID: origin.ID}
	if err := s.DB().Create(&domain).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplicationEndpoint(&model.ApplicationEndpoint{ApplicationID: app.ID, DomainID: domain.ID, Domain: domain.Hostname, ServicePort: 80}); err != nil {
		t.Fatal(err)
	}
	if err := s.AdoptApplicationNamespace(move); !errors.Is(err, model.ErrApplicationNamespaceMoveChanged) {
		t.Fatalf("accepted an unverified domain: %v", err)
	}
	move.DomainIDs = []uint{domain.ID}
	if err := s.AdoptApplicationNamespace(move); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetApplication(app.ID)
	if err != nil || got.EnvironmentID != destination.ID || got.ProjectID != target.ID || got.ManagementProjectID != nil {
		t.Fatalf("adopted application: %+v, %v", got, err)
	}
	binding, err := s.GetManagedDomain(domain.ID)
	if err != nil || binding.EnvironmentID != destination.ID || binding.Namespace != destination.Namespace {
		t.Fatalf("domain: %+v, %v", binding, err)
	}
	history, err := s.ListReleases(app.ID)
	if err != nil || len(history) != 1 || history[0].ID != release.ID {
		t.Fatalf("release history: %+v, %v", history, err)
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 2, ExpectedEnvironmentID: origin.ID}); !errors.Is(err, model.ErrApplicationNamespaceMoveChanged) {
		t.Fatalf("accepted release submitted against old namespace: %v", err)
	}
}
