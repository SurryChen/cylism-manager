package application

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type fakeApplier struct {
	preflightErr error
	applyErr     error
	readyErr     error
	applied      bool
}

func (a *fakeApplier) Preflight(context.Context, EndpointSpec) error { return a.preflightErr }
func (a *fakeApplier) Apply(context.Context, *RenderedResources) error {
	a.applied = true
	return a.applyErr
}
func (a *fakeApplier) WaitReady(context.Context, ApplicationContext, ReleaseSpec) error {
	return a.readyErr
}

func TestExecuteReleaseRecordsSuccessfulSteps(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createTestApplication(t, st)
	applier := &fakeApplier{}
	service := NewService(st, applier)
	release, err := service.CreateRelease(context.Background(), app.ID, 1, validTestReleaseSpec())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := service.ExecuteRelease(context.Background(), release.ID, app, validTestReleaseSpec()); err != nil {
		t.Fatalf("ExecuteRelease: %v", err)
	}
	stored, err := st.GetRelease(release.ID)
	if err != nil || stored.Status != model.ReleaseStatusSucceeded || !applier.applied {
		t.Fatalf("expected succeeded release, got %+v err=%v", stored, err)
	}
	operations, err := st.ListReleaseOperations(release.ID)
	if err != nil || len(operations) != 3 {
		t.Fatalf("expected three release operations, got %+v err=%v", operations, err)
	}
	operationLogs, err := st.ListOperationsByResource("release", release.ID)
	if err != nil || len(operationLogs) != 3 {
		t.Fatalf("expected three operation logs, got %+v err=%v", operationLogs, err)
	}
}

func TestExecuteReleaseMarksFailure(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{readyErr: context.DeadlineExceeded})
	release, err := service.CreateRelease(context.Background(), app.ID, 1, validTestReleaseSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), release.ID, app, validTestReleaseSpec()); err == nil {
		t.Fatal("expected readiness failure")
	}
	stored, _ := st.GetRelease(release.ID)
	if stored.Status != model.ReleaseStatusFailed {
		t.Fatalf("expected failed release, got %s", stored.Status)
	}
}

func TestRetryAndRollbackUseSanitizedSnapshot(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{})
	spec := validTestReleaseSpec()
	spec.Secrets = map[string]string{"DATABASE_PASSWORD": "secret"}
	first, err := service.CreateRelease(context.Background(), app.ID, 1, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), first.ID, app, spec); err != nil {
		t.Fatal(err)
	}
	failed, err := service.CreateRelease(context.Background(), app.ID, 1, validTestReleaseSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), failed.ID, app, validTestReleaseSpec()); err != nil {
		t.Fatal(err)
	}

	retry, retrySpec, err := service.RetryRelease(failed.ID, 1)
	if err != nil || retry.Sequence != 3 || retrySpec.Secrets["DATABASE_PASSWORD"] != "" {
		t.Fatalf("unexpected retry: %+v %+v err=%v", retry, retrySpec, err)
	}
	rollback, rollbackSpec, err := service.RollbackRelease(failed.ID, 1)
	if err != nil || rollback.SourceReleaseID == nil || *rollback.SourceReleaseID != first.ID || rollbackSpec.Image != spec.Image {
		t.Fatalf("unexpected rollback: %+v %+v err=%v", rollback, rollbackSpec, err)
	}
}

func createTestApplication(t *testing.T, st *store.Store) *model.Application {
	t.Helper()
	project := &model.Project{Name: "commerce", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	environment := &model.Environment{ProjectID: project.ID, Name: "production", Namespace: "commerce-prod"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	app := &model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "order-api", WorkloadKind: "deployment", CreatedBy: 1}
	if err := st.CreateApplication(app); err != nil {
		t.Fatal(err)
	}
	app, err := st.GetApplication(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func validTestReleaseSpec() ReleaseSpec {
	return ReleaseSpec{
		Image: "nginx:1.27", ContainerPort: 8080, Replicas: 1,
		Resources: ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
		Health:    HealthSpec{ReadinessPath: "/ready", LivenessPath: "/live"},
		Service:   ServiceSpec{Port: 80, TargetPort: 8080}, Endpoint: EndpointSpec{Exposure: ExposureCluster},
	}
}
