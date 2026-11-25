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
	metadata := model.Application{Name: "udp", ProjectID: source.ID, EnvironmentID: origin.ID, ManagementProjectID: &target.ID}
	if err := s.CreateApplication(&metadata); err != nil {
		t.Fatal(err)
	}
	metadataRelease := model.Release{ApplicationID: metadata.ID, Sequence: 1, Status: model.ReleaseStatusSucceeded}
	if err := s.CreateRelease(&metadataRelease); err != nil {
		t.Fatal(err)
	}
	metadataEndpoint := &model.ApplicationEndpoint{ApplicationID: metadata.ID, DomainID: domain.ID, Domain: domain.Hostname, ServicePort: 443, Protocol: "UDP"}
	if err := s.CreateApplicationEndpoint(metadataEndpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlanApplicationNamespaceMoveDomains(move); err == nil {
		t.Fatal("accepted a second Ingress application for a shared domain")
	}
	metadataEndpoint.IngressMode = "metadata"
	if err := s.UpdateApplicationEndpoint(metadataEndpoint); err != nil {
		t.Fatal(err)
	}
	metadataMove := move
	metadataMove.ApplicationID, metadataMove.ReleaseID = metadata.ID, metadataRelease.ID
	if ids, err := s.PlanApplicationNamespaceMoveDomains(metadataMove); err != nil || len(ids) != 0 {
		t.Fatalf("metadata binding must keep the domain with its Ingress: %v, %v", ids, err)
	}
	if err := s.AdoptApplicationNamespace(metadataMove); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetManagedDomain(domain.ID); err != nil || got.EnvironmentID != origin.ID {
		t.Fatalf("migrating a metadata binding moved its Ingress domain: %+v, %v", got, err)
	}
	if ids, err := s.PlanApplicationNamespaceMoveDomains(move); err != nil || len(ids) != 1 || ids[0] != domain.ID {
		t.Fatalf("Ingress owner should take its shared domain: %v, %v", ids, err)
	}

	if err := s.AdoptApplicationNamespace(move); !errors.Is(err, model.ErrApplicationNamespaceMoveChanged) {
		t.Fatalf("accepted an unverified domain: %v", err)
	}
	move.DomainIDs = []uint{domain.ID}
	task, err := s.BeginApplicationNamespaceMaintenance(move, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginApplicationNamespaceMaintenance(move, "{}"); err == nil {
		t.Fatal("accepted a concurrent migration")
	}
	if err := s.CreateRelease(&model.Release{ApplicationID: app.ID, Sequence: 2, ExpectedEnvironmentID: origin.ID}); !errors.Is(err, model.ErrApplicationNamespaceMaintenanceActive) {
		t.Fatalf("accepted a concurrent release: %v", err)
	}
	if err := s.DeleteEnvironment(destination.ID); !errors.Is(err, model.ErrApplicationNamespaceMaintenanceActive) {
		t.Fatalf("accepted deletion of the target environment: %v", err)
	}
	if err := s.AdoptApplicationNamespace(move); !errors.Is(err, model.ErrApplicationNamespaceMaintenanceActive) {
		t.Fatalf("accepted adoption outside the maintenance flow: %v", err)
	}
	move.MaintenanceID = task.ID
	if err := s.AdoptApplicationNamespace(move); !errors.Is(err, model.ErrApplicationNamespaceMoveChanged) {
		t.Fatalf("accepted adoption before route verification: %v", err)
	}
	if err := s.UpdateApplicationNamespaceMaintenance(task, model.NamespaceMoveRouted, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AdoptApplicationNamespace(move); err != nil {
		t.Fatal(err)
	}
	if active, err := s.GetApplicationNamespaceMaintenance(app.ID); err != nil || active != nil {
		t.Fatalf("adoption left an active maintenance checkpoint: %+v, %v", active, err)
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
