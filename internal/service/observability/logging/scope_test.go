package logging

import (
	"github.com/cylism/cylism-manager/internal/model"
	"testing"
)

type fakeScope struct{}

func (fakeScope) GetApplication(uint) (*model.Application, error) {
	return &model.Application{ProjectID: 2, EnvironmentID: 3, Name: "api", Environment: model.Environment{Namespace: "ns"}}, nil
}
func (fakeScope) GetEnvironmentByID(uint) (*model.Environment, error) {
	return &model.Environment{ID: 3, ProjectID: 2, Namespace: "ns"}, nil
}
func (fakeScope) GetProject(uint) (*model.Project, error) { return &model.Project{}, nil }
func TestResolveApplicationScopeFillsSelectors(t *testing.T) {
	req := QueryRequest{ApplicationID: 1}
	if err := ResolveApplicationScope(&req, fakeScope{}); err != nil {
		t.Fatal(err)
	}
	if req.Namespace != "ns" || req.Workload != "api" || req.ProjectID != 2 || req.EnvironmentID != 3 {
		t.Fatalf("unexpected %#v", req)
	}
}
