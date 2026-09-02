package application

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/store"
)

type fakeApplier struct {
	verifyErr    error
	preflightErr error
	applyErr     error
	readyErr     error
	verified     bool
	applied      bool
}

func (a *fakeApplier) VerifyImage(context.Context, ReleaseSpec) error {
	a.verified = true
	return a.verifyErr
}
func (a *fakeApplier) Preflight(context.Context, ApplicationContext, ReleaseSpec) error {
	return a.preflightErr
}
func (a *fakeApplier) Apply(context.Context, *RenderedResources) error {
	a.applied = true
	return a.applyErr
}
func (a *fakeApplier) WaitReady(context.Context, ApplicationContext, ReleaseSpec) error {
	return a.readyErr
}

type releaseRepositoryStub struct {
	application *model.Application
	release     *model.Release
	releases    []model.Release
}

var _ repository.ReleaseRepository = (*releaseRepositoryStub)(nil)

func (s *releaseRepositoryStub) GetApplication(uint) (*model.Application, error) {
	return s.application, nil
}

func (s *releaseRepositoryStub) ListReleases(uint) ([]model.Release, error) {
	return append([]model.Release(nil), s.releases...), nil
}

func (s *releaseRepositoryStub) CreateRelease(release *model.Release) error {
	release.ID = uint(len(s.releases) + 1)
	s.release = release
	s.releases = append(s.releases, *release)
	return nil
}

func (s *releaseRepositoryStub) GetRelease(uint) (*model.Release, error) {
	return s.release, nil
}

func (s *releaseRepositoryStub) UpdateRelease(release *model.Release) error {
	s.release = release
	return nil
}

func (*releaseRepositoryStub) CreateReleaseOperation(*model.ReleaseOperation) error { return nil }
func (*releaseRepositoryStub) UpdateReleaseOperation(*model.ReleaseOperation) error { return nil }
func (*releaseRepositoryStub) CreateOperationLog(*model.OperationLog) error         { return nil }
func (*releaseRepositoryStub) UpdateOperationLog(*model.OperationLog) error         { return nil }

func TestCreateReleaseUsesRepositoryStub(t *testing.T) {
	repository := &releaseRepositoryStub{application: &model.Application{
		ID: 1, Name: "order-api", WorkloadKind: WorkloadKindDeployment,
		Project:     model.Project{ID: 2, Name: "commerce"},
		Environment: model.Environment{ID: 3, Name: "production", Namespace: "commerce-production"},
	}}
	release, err := NewService(repository, &fakeApplier{}).CreateRelease(context.Background(), 1, 9, validTestReleaseSpec())
	if err != nil {
		t.Fatalf("CreateRelease() error = %v", err)
	}
	if repository.release != release || release.Sequence != 1 || release.CreatedBy != 9 || release.Status != model.ReleaseStatusDraft {
		t.Fatalf("unexpected release persisted by repository: %#v", release)
	}
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
	if err != nil || stored.Status != model.ReleaseStatusSucceeded || !applier.verified || !applier.applied {
		t.Fatalf("expected succeeded release, got %+v err=%v", stored, err)
	}
	operations, err := st.ListReleaseOperations(release.ID)
	if err != nil || len(operations) != 4 {
		t.Fatalf("expected four release operations, got %+v err=%v", operations, err)
	}
	operationLogs, err := st.ListOperationsByResource("release", release.ID)
	if err != nil || len(operationLogs) != 4 {
		t.Fatalf("expected four operation logs, got %+v err=%v", operationLogs, err)
	}
}

func TestExecuteReleaseStopsBeforeApplyWhenImageVerificationFails(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	applier := &fakeApplier{verifyErr: context.DeadlineExceeded}
	service := NewService(st, applier)
	release, err := service.CreateRelease(context.Background(), app.ID, 1, validTestReleaseSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), release.ID, app, validTestReleaseSpec()); err == nil {
		t.Fatal("expected image verification failure")
	}
	if applier.applied {
		t.Fatal("image verification failure must prevent resource application")
	}
	operations, err := st.ListReleaseOperations(release.ID)
	if err != nil || len(operations) != 1 || operations[0].Step != "verify_image" || operations[0].Status != model.ReleaseOperationFailed {
		t.Fatalf("unexpected operations: %+v err=%v", operations, err)
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

func TestCreateReleasePersistsRegistryReference(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{})
	spec := validTestReleaseSpec()
	spec.RegistryID = 12
	release, err := service.CreateRelease(context.Background(), app.ID, 1, spec)
	if err != nil || release.ImageRegistryID == nil || *release.ImageRegistryID != 12 || !release.PodTrackingEnabled {
		t.Fatalf("expected registry reference to be persisted, got %+v err=%v", release, err)
	}
}

func TestRetryAndRollbackUseSanitizedSnapshot(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{})
	spec := validTestReleaseSpec()
	spec.Secrets = map[string]string{"DATABASE_PASSWORD": "secret"}
	spec.Endpoint = EndpointSpec{Exposure: ExposurePublic, Domain: "api.example.com", Path: "/", TLSEnabled: true, IssuerRef: "letsencrypt-prod"}
	first, err := service.CreateRelease(context.Background(), app.ID, 1, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), first.ID, app, spec); err != nil {
		t.Fatal(err)
	}
	legacySpec := validTestReleaseSpec()
	legacySpec.Endpoint = EndpointSpec{Exposure: ExposurePublic, Domain: "legacy.example.com", Path: "/", TLSEnabled: true, IssuerRef: "letsencrypt-prod"}
	failed, err := service.CreateRelease(context.Background(), app.ID, 1, legacySpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteRelease(context.Background(), failed.ID, app, legacySpec); err != nil {
		t.Fatal(err)
	}

	retry, retrySpec, err := service.RetryRelease(context.Background(), failed.ID, 1)
	if err != nil || retry.Sequence != 3 || retrySpec.Secrets["DATABASE_PASSWORD"] != "" || retrySpec.Endpoint.Exposure != ExposureCluster {
		t.Fatalf("unexpected retry: %+v %+v err=%v", retry, retrySpec, err)
	}
	rollback, rollbackSpec, err := service.RollbackRelease(context.Background(), failed.ID, 1)
	if err != nil || rollback.SourceReleaseID == nil || *rollback.SourceReleaseID != first.ID || rollbackSpec.Image != spec.Image || rollbackSpec.Endpoint.Exposure != ExposureCluster {
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
