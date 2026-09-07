package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestProjectAndEnvironmentCRUD(t *testing.T) {
	st := setupTestDB(t)
	project := &model.Project{Name: "commerce", Description: "订单服务", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "staging", Namespace: "commerce-staging"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	projects, err := st.ListProjects()
	if err != nil || len(projects) != 1 || len(projects[0].Environments) != 1 {
		t.Fatalf("ListProjects should preload environments, got %+v, err=%v", projects, err)
	}

	project.Description = "订单服务与部署环境"
	if err := st.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	environment.Namespace = "commerce-preview"
	if err := st.UpdateEnvironment(environment); err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}

	if err := st.DeleteEnvironment(environment.ID); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if err := st.DeleteProject(project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

func TestEnvironmentNamespaceIsGloballyUnique(t *testing.T) {
	st := setupTestDB(t)
	first := &model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce"}
	if err := st.CreateEnvironment(first); err != nil {
		t.Fatalf("create first environment: %v", err)
	}
	second := &model.Environment{ProjectID: 2, Name: "production", Namespace: "commerce"}
	err := st.CreateEnvironment(second)
	var conflict *model.NamespaceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected NamespaceConflictError, got %v", err)
	}
	if conflict.EnvironmentID != first.ID || conflict.ProjectID != first.ProjectID {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
}

func TestReconcileEnvironmentNamespaceUniquenessWaitsForLegacyConflicts(t *testing.T) {
	st := setupTestDB(t)
	if err := st.db.Exec("DROP INDEX IF EXISTS idx_environments_namespace_unique").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.db.Exec("INSERT INTO environments (project_id, name, namespace, created_at, updated_at) VALUES (1, 'one', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (2, 'two', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error; err != nil {
		t.Fatal(err)
	}
	if err := st.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		t.Fatalf("reconcile with legacy conflicts: %v", err)
	}
	conflicts, err := st.ListEnvironmentNamespaceConflicts()
	if err != nil || len(conflicts) != 1 || conflicts[0].Namespace != "shared" || len(conflicts[0].Environments) != 2 {
		t.Fatalf("unexpected conflicts: %#v, err=%v", conflicts, err)
	}
	if err := st.db.Where("project_id = ?", 2).Delete(&model.Environment{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.ReconcileEnvironmentNamespaceUniqueness(); err != nil {
		t.Fatalf("reconcile resolved conflicts: %v", err)
	}
	if err := st.db.Exec("INSERT INTO environments (project_id, name, namespace, created_at, updated_at) VALUES (3, 'three', 'shared', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error; err == nil {
		t.Fatal("expected unique index to reject duplicate namespace")
	}
}

func TestBackfillManagedDomainEnvironmentWhenNamespaceHasSingleOwner(t *testing.T) {
	st := setupTestDB(t)
	environment := &model.Environment{ProjectID: 1, Name: "production", Namespace: "commerce-prod"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	domain := &model.ManagedDomain{Hostname: "api.example.com", Namespace: environment.Namespace}
	if err := st.CreateManagedDomain(domain); err != nil {
		t.Fatal(err)
	}
	if err := st.backfillManagedDomainEnvironments(); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetManagedDomain(domain.ID)
	if err != nil || updated.EnvironmentID != environment.ID {
		t.Fatalf("expected domain to bind environment %d, got %#v err=%v", environment.ID, updated, err)
	}
}

func TestApplicationEndpointsSupportIndependentUpdatesAndExactRouteExclusion(t *testing.T) {
	st := setupTestDB(t)
	first := &model.ApplicationEndpoint{ApplicationID: 1, DomainID: 7, Exposure: "public", Domain: "api.example.com", Path: "/", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(first); err != nil {
		t.Fatal(err)
	}
	second := &model.ApplicationEndpoint{ApplicationID: 1, DomainID: 8, Exposure: "public", Domain: "admin.example.com", Path: "/", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(second); err != nil {
		t.Fatal(err)
	}
	third := &model.ApplicationEndpoint{ApplicationID: 2, DomainID: 7, Exposure: "public", Domain: "api.example.com", Path: "/health", ServicePort: 80}
	if err := st.CreateApplicationEndpoint(third); err != nil {
		t.Fatal(err)
	}

	endpoints, err := st.ListApplicationEndpoints(1)
	if err != nil || len(endpoints) != 2 || endpoints[0].ID != first.ID || endpoints[1].ID != second.ID {
		t.Fatalf("expected two ordered endpoints, got %#v err=%v", endpoints, err)
	}
	second.Path = "/admin"
	if err := st.UpdateApplicationEndpoint(second); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetApplicationEndpoint(1, second.ID)
	if err != nil || updated.Path != "/admin" {
		t.Fatalf("expected independently updated endpoint, got %#v err=%v", updated, err)
	}

	count, err := st.CountApplicationEndpointRoute(7, "/", first.ID)
	if err != nil || count != 0 {
		t.Fatalf("expected current endpoint route to be excluded, count=%d err=%v", count, err)
	}
	count, err = st.CountApplicationEndpointRoute(7, "/", second.ID)
	if err != nil || count != 1 {
		t.Fatalf("expected another endpoint in same application to conflict, count=%d err=%v", count, err)
	}
	count, err = st.CountApplicationEndpointRoute(7, "/health", first.ID)
	if err != nil || count != 1 {
		t.Fatalf("expected conflicting route, count=%d err=%v", count, err)
	}
	if err := st.DeleteApplicationEndpoint(1, first.ID); err != nil {
		t.Fatal(err)
	}
	endpoints, err = st.ListApplicationEndpoints(1)
	if err != nil || len(endpoints) != 1 || endpoints[0].ID != second.ID {
		t.Fatalf("expected only second endpoint after delete, got %#v err=%v", endpoints, err)
	}
}
