package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	applicationdomain "github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

var (
	ErrWorkloadSnapshot    = errors.New("workload snapshot read")
	ErrWorkloadPersistence = errors.New("workload persistence")
)

// WorkloadMigrator is the Kubernetes capability needed to switch an
// application's workload kind while preserving its last successful spec.
type WorkloadMigrator interface {
	Preflight(context.Context, applicationdomain.ApplicationContext, applicationdomain.ReleaseSpec) error
	MigrateWorkloadKind(context.Context, applicationdomain.ApplicationContext, applicationdomain.ReleaseSpec, string) error
}

type WorkloadService struct {
	management interface {
		GetLatestSuccessfulRelease(uint) (*model.Release, error)
		UpdateApplication(*model.Application) error
	}
	migrator WorkloadMigrator
}

func NewWorkloadService(management interface {
	GetLatestSuccessfulRelease(uint) (*model.Release, error)
	UpdateApplication(*model.Application) error
}, migrator WorkloadMigrator) *WorkloadService {
	return &WorkloadService{management: management, migrator: migrator}
}

// UpdateKind changes the persisted workload kind and migrates the active
// Kubernetes workload when a successful release already exists.
func (s *WorkloadService) UpdateKind(ctx context.Context, app *model.Application, target string) error {
	if app == nil {
		return errors.New("application is nil")
	}
	if app.WorkloadKind == target {
		return nil
	}
	release, err := s.management.GetLatestSuccessfulRelease(app.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		app.WorkloadKind = target
		if err := s.management.UpdateApplication(app); err != nil {
			return fmt.Errorf("%w: %v", ErrWorkloadPersistence, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWorkloadPersistence, err)
	}
	var spec applicationdomain.ReleaseSpec
	if err := json.Unmarshal([]byte(release.DesiredSpec), &spec); err != nil {
		return fmt.Errorf("%w: 读取成功发布快照失败", ErrWorkloadSnapshot)
	}
	appContext := applicationContextFor(app)
	appContext.ReleaseSequence = release.Sequence
	if s.migrator == nil {
		return errors.New("kubernetes migrator unavailable")
	}
	if err := s.migrator.Preflight(ctx, appContext, spec); err != nil {
		return err
	}
	if err := s.migrator.MigrateWorkloadKind(ctx, appContext, spec, target); err != nil {
		return err
	}
	app.WorkloadKind = target
	if err := s.management.UpdateApplication(app); err != nil {
		return fmt.Errorf("%w: %v", ErrWorkloadPersistence, err)
	}
	return nil
}

func applicationContextFor(app *model.Application) applicationdomain.ApplicationContext {
	return applicationdomain.ApplicationContext{ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, ApplicationName: app.Name, Namespace: app.Environment.Namespace, WorkloadKind: app.WorkloadKind}
}
