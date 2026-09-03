package application

import (
	"context"

	applicationdomain "github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

// ReleaseService is the application-service boundary for release use cases.
// ReleaseWorkflow remains the domain implementation; this service makes its
// composition explicit and keeps HTTP handlers out of the workflow package.
type ReleaseService struct {
	workflow *applicationdomain.ReleaseWorkflow
}

func NewReleaseService(repository repository.ReleaseWorkflowRepository, encKey []byte, applier applicationdomain.ResourceApplier) *ReleaseService {
	return &ReleaseService{workflow: applicationdomain.NewReleaseWorkflow(repository, encKey, applier)}
}

func (s *ReleaseService) CreateFromTemplate(ctx context.Context, app *model.Application, template *model.ApplicationDeploymentTemplate, version string, userID uint) (*applicationdomain.PreparedRelease, error) {
	return s.workflow.CreateFromTemplate(ctx, app, template, version, userID)
}

func (s *ReleaseService) Restart(ctx context.Context, app *model.Application, userID uint) (*applicationdomain.PreparedRelease, error) {
	return s.workflow.Restart(ctx, app, userID)
}

func (s *ReleaseService) Retry(ctx context.Context, releaseID, userID uint) (*applicationdomain.PreparedRelease, error) {
	return s.workflow.Retry(ctx, releaseID, userID)
}

func (s *ReleaseService) Rollback(ctx context.Context, releaseID, userID uint) (*applicationdomain.PreparedRelease, error) {
	return s.workflow.Rollback(ctx, releaseID, userID)
}

func (s *ReleaseService) ExecuteAsync(ctx context.Context, app *model.Application, prepared *applicationdomain.PreparedRelease) {
	s.workflow.ExecuteAsync(ctx, app, prepared)
}

func (s *ReleaseService) SyncApplicationEndpoints(ctx context.Context, app *model.Application, service applicationdomain.ServiceSpec) error {
	return s.workflow.SyncApplicationEndpoints(ctx, app, service)
}

func (s *ReleaseService) SyncManagedFiles(app *model.Application, spec applicationdomain.ReleaseSpec, userID uint) error {
	return s.workflow.SyncManagedFiles(app, spec, userID)
}

func (s *ReleaseService) PrepareRegistrySpec(app *model.Application, spec *applicationdomain.ReleaseSpec) error {
	return s.workflow.PrepareRegistrySpec(app, spec)
}
