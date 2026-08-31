// Package application contains read-only application queries used by API
// views and integration endpoints. Mutation workflows remain in the API and
// release service layers.
package application

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

var ErrEnvironmentNotInProject = errors.New("environment does not belong to project")

// QueryService provides the shared read operations used by application views.
// It deliberately has no mutation or Kubernetes dependencies.
type QueryService struct {
	repository repository.ApplicationQueryRepository
}

func NewQueryService(repository repository.ApplicationQueryRepository) *QueryService {
	return &QueryService{repository: repository}
}

func (s *QueryService) GetProject(id uint) (*model.Project, error) {
	return s.repository.GetProject(id)
}

func (s *QueryService) GetEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	return s.repository.GetEnvironment(projectID, environmentID)
}

// ResolveEnvironment returns an environment only when it belongs to the
// supplied project. Callers use the sentinel error to map an invalid project /
// environment pair to their existing API response.
func (s *QueryService) ResolveEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	environment, err := s.repository.GetEnvironment(projectID, environmentID)
	if err != nil {
		return nil, ErrEnvironmentNotInProject
	}
	return environment, nil
}

func (s *QueryService) GetApplication(id uint) (*model.Application, error) {
	return s.repository.GetApplication(id)
}

func (s *QueryService) ListApplications(scope ...uint) ([]model.Application, error) {
	return s.repository.ListApplications(scope...)
}

func (s *QueryService) ListReleases(applicationID uint) ([]model.Release, error) {
	return s.repository.ListReleases(applicationID)
}

func (s *QueryService) ListReleasesByApplications(ids []uint) (map[uint][]model.Release, error) {
	return s.repository.ListReleasesByApplications(ids)
}
