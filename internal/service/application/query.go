// Package application contains read-only application queries used by API
// views and integration endpoints. Mutation workflows remain in the API and
// release service layers.
package application

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
)

var ErrEnvironmentNotInProject = errors.New("environment does not belong to project")

// Store is the smallest read model needed by QueryService. Keeping this
// interface here lets query consumers use a stub without depending on the
// concrete database Store.
type Store interface {
	GetProject(id uint) (*model.Project, error)
	GetEnvironment(projectID, environmentID uint) (*model.Environment, error)
	GetApplication(id uint) (*model.Application, error)
	ListApplications(scope ...uint) ([]model.Application, error)
	ListReleases(applicationID uint) ([]model.Release, error)
	ListReleasesByApplications(ids []uint) (map[uint][]model.Release, error)
}

// QueryService provides the shared read operations used by application views.
// It deliberately has no mutation or Kubernetes dependencies.
type QueryService struct{ store Store }

func NewQueryService(store Store) *QueryService { return &QueryService{store: store} }

func (s *QueryService) GetProject(id uint) (*model.Project, error) {
	return s.store.GetProject(id)
}

func (s *QueryService) GetEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	return s.store.GetEnvironment(projectID, environmentID)
}

// ResolveEnvironment returns an environment only when it belongs to the
// supplied project. Callers use the sentinel error to map an invalid project /
// environment pair to their existing API response.
func (s *QueryService) ResolveEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	environment, err := s.store.GetEnvironment(projectID, environmentID)
	if err != nil {
		return nil, ErrEnvironmentNotInProject
	}
	return environment, nil
}

func (s *QueryService) GetApplication(id uint) (*model.Application, error) {
	return s.store.GetApplication(id)
}

func (s *QueryService) ListApplications(scope ...uint) ([]model.Application, error) {
	return s.store.ListApplications(scope...)
}

func (s *QueryService) ListReleases(applicationID uint) ([]model.Release, error) {
	return s.store.ListReleases(applicationID)
}

func (s *QueryService) ListReleasesByApplications(ids []uint) (map[uint][]model.Release, error) {
	return s.store.ListReleasesByApplications(ids)
}
