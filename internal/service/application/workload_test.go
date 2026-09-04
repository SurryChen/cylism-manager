package application

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

type workloadManagementFake struct {
	release *model.Release
	app     *model.Application
}

func (f *workloadManagementFake) GetLatestSuccessfulRelease(uint) (*model.Release, error) {
	if f.release == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.release, nil
}
func (f *workloadManagementFake) UpdateApplication(app *model.Application) error {
	f.app = app
	return nil
}

type workloadMigratorFake struct{ preflight, migrate bool }

func (f *workloadMigratorFake) Preflight(context.Context, ApplicationContext, ReleaseSpec) error {
	f.preflight = true
	return nil
}
func (f *workloadMigratorFake) MigrateWorkloadKind(context.Context, ApplicationContext, ReleaseSpec, string) error {
	f.migrate = true
	return nil
}

func TestWorkloadServiceUpdatesWithoutRelease(t *testing.T) {
	fake := &workloadManagementFake{}
	svc := NewWorkloadService(fake, nil)
	app := &model.Application{ID: 1, WorkloadKind: WorkloadKindDeployment}
	if err := svc.UpdateKind(context.Background(), app, WorkloadKindStatefulSet); err != nil {
		t.Fatal(err)
	}
	if fake.app != app || app.WorkloadKind != WorkloadKindStatefulSet {
		t.Fatalf("application was not updated: %#v", fake.app)
	}
}

func TestWorkloadServiceMigratesSuccessfulRelease(t *testing.T) {
	fake := &workloadManagementFake{release: &model.Release{Sequence: 3, DesiredSpec: `{}`}}
	migrator := &workloadMigratorFake{}
	svc := NewWorkloadService(fake, migrator)
	app := &model.Application{ID: 1, Name: "demo", WorkloadKind: WorkloadKindDeployment, Environment: model.Environment{Namespace: "ns"}}
	if err := svc.UpdateKind(context.Background(), app, WorkloadKindStatefulSet); err != nil {
		t.Fatal(err)
	}
	if !migrator.preflight || !migrator.migrate || fake.app != app {
		t.Fatal("expected migration and persistence")
	}
}
