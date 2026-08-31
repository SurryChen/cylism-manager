package application

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type queryStoreStub struct{}

func (queryStoreStub) GetProject(id uint) (*model.Project, error) {
	return &model.Project{ID: id, Name: "demo"}, nil
}
func (queryStoreStub) GetEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	if projectID != 7 || environmentID != 8 {
		return nil, errors.New("not found")
	}
	return &model.Environment{ID: environmentID, ProjectID: projectID, Name: "dev"}, nil
}
func (queryStoreStub) GetApplication(id uint) (*model.Application, error) {
	return &model.Application{ID: id, Name: "app"}, nil
}
func (queryStoreStub) ListApplications(scope ...uint) ([]model.Application, error) {
	var projectID, environmentID uint
	if len(scope) > 0 {
		projectID = scope[0]
	}
	if len(scope) > 1 {
		environmentID = scope[1]
	}
	return []model.Application{{ID: 1, ProjectID: projectID, EnvironmentID: environmentID, Name: "app"}}, nil
}
func (queryStoreStub) ListReleases(applicationID uint) ([]model.Release, error) {
	return []model.Release{{ApplicationID: applicationID, Sequence: 1}}, nil
}
func (queryStoreStub) ListReleasesByApplications(ids []uint) (map[uint][]model.Release, error) {
	return map[uint][]model.Release{ids[0]: {{ApplicationID: ids[0], Sequence: 1}}}, nil
}

func TestQueryServiceDelegatesReadModelOperations(t *testing.T) {
	service := NewQueryService(queryStoreStub{})
	if project, err := service.GetProject(7); err != nil || project.ID != 7 {
		t.Fatalf("GetProject() = %#v, %v", project, err)
	}
	if environment, err := service.GetEnvironment(7, 8); err != nil || environment.ProjectID != 7 || environment.ID != 8 {
		t.Fatalf("GetEnvironment() = %#v, %v", environment, err)
	}
	if environment, err := service.ResolveEnvironment(7, 8); err != nil || environment.ProjectID != 7 || environment.ID != 8 {
		t.Fatalf("ResolveEnvironment() = %#v, %v", environment, err)
	}
	if environment, err := service.ResolveEnvironment(7, 9); !errors.Is(err, ErrEnvironmentNotInProject) || environment != nil {
		t.Fatalf("ResolveEnvironment() outside project = %#v, %v", environment, err)
	}
	if app, err := service.GetApplication(9); err != nil || app.ID != 9 {
		t.Fatalf("GetApplication() = %#v, %v", app, err)
	}
	applications, err := service.ListApplications(7, 8)
	if err != nil || len(applications) != 1 {
		t.Fatalf("ListApplications() = %#v, %v", applications, err)
	}
	releases, err := service.ListReleases(9)
	if err != nil || len(releases) != 1 {
		t.Fatalf("ListReleases() = %#v, %v", releases, err)
	}
	grouped, err := service.ListReleasesByApplications([]uint{9})
	if err != nil || len(grouped[9]) != 1 {
		t.Fatalf("ListReleasesByApplications() = %#v, %v", grouped, err)
	}
}
