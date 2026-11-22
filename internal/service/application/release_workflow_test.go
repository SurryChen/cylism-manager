package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/store"
)

func TestReleaseWorkflowCreatesReleaseFromTemplateWithRuntimeCredentials(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createTestApplication(t, st)
	key := []byte("01234567890123456789012345678901")
	credential, err := crypto.Encrypt(key, "registry-password")
	if err != nil {
		t.Fatal(err)
	}
	registry := &model.ImageRegistry{
		Name: "private", Endpoint: "registry.example.com", AuthType: "basic", Username: "deploy", Credential: credential, Enabled: true,
	}
	if err := st.CreateImageRegistry(registry, []uint{app.ProjectID}); err != nil {
		t.Fatal(err)
	}
	spec := validTestReleaseSpec()
	ndots := 1
	spec.DNSNDots = &ndots
	spec.Image = "order-api"
	spec.RegistryID = registry.ID
	spec.Secrets = map[string]string{"API_TOKEN": ""}
	templateSpec, err := json.Marshal(SanitizeReleaseSpec(spec))
	if err != nil {
		t.Fatal(err)
	}
	encryptedSecrets, err := crypto.Encrypt(key, `{"API_TOKEN":"template-secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{
		ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(templateSpec), EncryptedSecrets: encryptedSecrets,
	}
	if err := st.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}

	workflow := NewReleaseWorkflow(st, key, &fakeApplier{})
	prepared, err := workflow.CreateFromTemplate(context.Background(), app, template, "1.2.3", 9)
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	if prepared.Release.Image != "registry.example.com/order-api:1.2.3" || prepared.Release.TemplateID == nil || *prepared.Release.TemplateID != template.ID {
		t.Fatalf("unexpected release: %#v", prepared.Release)
	}
	if prepared.Spec.RegistryCredential != "registry-password" || prepared.Spec.Secrets["API_TOKEN"] != "template-secret" {
		t.Fatalf("runtime credentials were not resolved: %#v", prepared.Spec)
	}
	var snapshot ReleaseSpec
	if err := json.Unmarshal([]byte(prepared.Release.DesiredSpec), &snapshot); err != nil {
		t.Fatal(err)
	}
	if prepared.Spec.DNSNDots == nil || *prepared.Spec.DNSNDots != 1 || snapshot.DNSNDots == nil || *snapshot.DNSNDots != 1 {
		t.Fatalf("template release lost DNS setting: runtime=%+v snapshot=%+v", prepared.Spec.DNSNDots, snapshot.DNSNDots)
	}
	if strings.Contains(prepared.Release.DesiredSpec, "template-secret") || strings.Contains(prepared.Release.DesiredSpec, "registry-password") {
		t.Fatalf("release snapshot leaked credentials: %s", prepared.Release.DesiredSpec)
	}
}

func TestReleaseWorkflowPreparesFixedNodeWithoutMirror(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	workflow := NewReleaseWorkflow(st, nil, &fakeApplier{})
	spec := ReleaseSpec{Image: "zenika/alpine-chrome:124", NodeName: "worker-a"}
	if err := workflow.PrepareRuntimeSpec(nil, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Image != "zenika/alpine-chrome:124" || spec.NodeName != "worker-a" {
		t.Fatalf("runtime spec changed unexpectedly: %+v", spec)
	}
}

func TestReleaseWorkflowRejectsMissingDependenciesBeforeCreatingRelease(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createTestApplication(t, st)
	spec := validTestReleaseSpec()
	spec.Image = "nginx"
	templateSpec, err := json.Marshal(SanitizeReleaseSpec(spec))
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Enabled: true, DefaultVersion: "1.0.0", Spec: string(templateSpec)}
	workflow := NewReleaseWorkflow(st, nil, &fakeApplier{preflightErr: context.DeadlineExceeded})
	if _, err := workflow.CreateFromTemplate(context.Background(), app, template, "", 1); err == nil || !strings.Contains(err.Error(), "发布前检查失败") {
		t.Fatalf("expected synchronous preflight error: %v", err)
	}
	releases, err := st.ListReleases(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 0 {
		t.Fatalf("preflight failure created %d release records", len(releases))
	}
}

func TestMigrationRetryAndRollbackRestoreOriginalTemplateSecrets(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createTestApplication(t, st)
	key := []byte("01234567890123456789012345678901")
	spec := testMigrationSpec()
	spec.Image = "ghcr.io/example/orders"
	snapshot, _ := json.Marshal(SanitizeReleaseSpec(spec))
	encrypted, _ := crypto.Encrypt(key, "{\"DATABASE_PASSWORD\":\"original-secret\"}")
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "original", Enabled: true, Spec: string(snapshot), EncryptedSecrets: encrypted}
	if err := st.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	workflow := NewReleaseWorkflow(st, key, &fakeApplier{})
	source, err := workflow.CreateFromTemplate(context.Background(), app, template, "1.0.0", 1, &MigrationRequest{Target: "202611220001"})
	if err != nil {
		t.Fatal(err)
	}
	source.Release.Status = model.ReleaseStatusSucceeded
	st.UpdateRelease(source.Release)
	otherSecrets, _ := crypto.Encrypt(key, "{\"DATABASE_PASSWORD\":\"wrong-secret\"}")
	other := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "new-default", Enabled: true, Spec: string(snapshot), EncryptedSecrets: otherSecrets}
	if err = st.CreateApplicationDeploymentTemplate(other, true); err != nil {
		t.Fatal(err)
	}
	retry, err := workflow.Retry(context.Background(), source.Release.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Spec.Secrets["DATABASE_PASSWORD"] != "original-secret" || retry.Spec.Migration.Target != "202611220001" {
		t.Fatal("retry changed source secrets or migration target")
	}
	retry.Release.Status = model.ReleaseStatusFailed
	st.UpdateRelease(retry.Release)
	rollback, err := workflow.Rollback(context.Background(), retry.Release.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Spec.Secrets["DATABASE_PASSWORD"] != "original-secret" || rollback.Spec.Migration != nil {
		t.Fatal("rollback changed secrets or replays SQL")
	}
	rollback.Release.Status = model.ReleaseStatusFailed
	st.UpdateRelease(rollback.Release)
	st.DB().Model(template).Update("encrypted_secrets", "")
	before, _ := st.ListReleases(app.ID)
	if _, err = workflow.Retry(context.Background(), source.Release.ID, 1); err == nil {
		t.Fatal("missing credentials accepted")
	}
	after, _ := st.ListReleases(app.ID)
	if len(before) != len(after) {
		t.Fatal("failed secret restoration created orphan draft")
	}
}

func TestMigrationReleaseCannotBeOvertaken(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{})
	if _, err := service.CreateRelease(context.Background(), app.ID, 1, testMigrationSpec()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateRelease(context.Background(), app.ID, 1, validTestReleaseSpec()); err == nil {
		t.Fatal("workload overtook migration")
	}
}
