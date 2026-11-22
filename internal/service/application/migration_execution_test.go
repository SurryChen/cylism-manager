package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func testMigrationSpec() ReleaseSpec {
	spec := validTestReleaseSpec()
	spec.Migration = &MigrationSpec{Enabled: true, Command: []string{"/usr/local/bin/cylism-migrate"}, Directory: "/app/migrations", DatabasePrefix: "DATABASE", TimeoutSeconds: 60, Target: "202611220001"}
	spec.Config = map[string]string{"DATABASE_HOST": "mysql.internal", "DATABASE_PORT": "3306", "DATABASE_NAME": "orders", "DATABASE_USER": "orders"}
	spec.Secrets = map[string]string{"DATABASE_PASSWORD": "private-password"}
	return spec
}

type migrationApplier struct {
	fakeApplier
	called bool
	err    error
}

func (a *migrationApplier) RunMigration(context.Context, ApplicationContext, ReleaseSpec) (string, error) {
	if a.applied {
		return "", errors.New("workload changed before migration")
	}
	a.called = true
	return "Migration 202611220001 completed", a.err
}
func TestMigrationFailureStopsBeforeWorkloadApply(t *testing.T) {
	for _, fail := range []bool{false, true} {
		st, _ := store.New(":memory:")
		app := createTestApplication(t, st)
		applier := &migrationApplier{}
		if fail {
			applier.err = errors.New("dirty database")
		}
		service := NewService(st, applier)
		spec := testMigrationSpec()
		release, err := service.CreateRelease(context.Background(), app.ID, 1, spec)
		if err != nil {
			t.Fatal(err)
		}
		err = service.ExecuteRelease(context.Background(), release.ID, app, spec)
		if (err != nil) != fail || !applier.called || applier.applied == fail {
			t.Fatalf("fail=%v applied=%v err=%v", fail, applier.applied, err)
		}
		operations, _ := st.ListReleaseOperations(release.ID)
		if operations[2].Step != "database_migration" {
			t.Fatalf("wrong order: %+v", operations)
		}
		if !fail && !strings.Contains(operations[2].Detail, "202611220001") {
			t.Fatal("successful migration output missing")
		}
	}
}
func TestMigrationJobIsIsolatedAndDoesNotAutomaticallyRetry(t *testing.T) {
	spec := testMigrationSpec()
	spec.Args = []string{"serve"}
	app := ApplicationContext{ProjectName: "p", EnvironmentName: "prod", Namespace: "apps", ApplicationName: "orders", ReleaseSequence: 2}
	job, resources, err := renderMigrationJob(app, spec)
	if err != nil {
		t.Fatal(err)
	}
	pod := job.Spec.Template.Spec
	container := pod.Containers[0]
	if *job.Spec.BackoffLimit != 0 || pod.RestartPolicy != corev1.RestartPolicyNever || *pod.AutomountServiceAccountToken || container.ReadinessProbe != nil || len(container.Args) > 0 {
		t.Fatalf("unsafe migration job: %+v", pod)
	}
	if resources.Secret.Name == "orders-secret" || resources.ConfigMap.Name == "orders-config" {
		t.Fatal("migration mutates live workload configuration")
	}
	if job.Name != migrationResourceName(app) || len(job.Name) > 63 {
		t.Fatal("job name not stable")
	}
	payload, _ := json.Marshal(job)
	if strings.Contains(string(payload), "private-password") {
		t.Fatal("Job leaked secret value")
	}
	if got := redactMigrationLog("failure private-password", spec); strings.Contains(got, "private-password") {
		t.Fatal("log leaked secret")
	}
	client := k8sfake.NewSimpleClientset()
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if _, err = client.BatchV1().Jobs("apps").Create(context.Background(), job, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	adapter := &KubernetesApplier{migrations: client}
	detail, err := adapter.RunMigration(context.Background(), app, spec)
	if err != nil || !strings.Contains(detail, "202611220001") {
		t.Fatalf("resume completed job: detail=%s err=%v", detail, err)
	}
}
func TestMigrationRequestCannotSilentlySkipEnabledTemplate(t *testing.T) {
	spec := testMigrationSpec()
	if err := bindMigrationRequest(&spec); err == nil {
		t.Fatal("omitted target bypassed migration")
	}
	if err := bindMigrationRequest(&spec, &MigrationRequest{Target: "0"}); err == nil {
		t.Fatal("zero target accepted")
	}
	if err := bindMigrationRequest(&spec, &MigrationRequest{Target: "202611220001"}); err != nil {
		t.Fatal(err)
	}
	spec.Migration = nil
	if err := bindMigrationRequest(&spec, &MigrationRequest{Target: "1"}); err == nil {
		t.Fatal("callback activated unconfigured command")
	}
}
func TestRollbackDropsMigrationTarget(t *testing.T) {
	st, _ := store.New(":memory:")
	app := createTestApplication(t, st)
	service := NewService(st, &fakeApplier{})
	source, err := service.CreateRelease(context.Background(), app.ID, 1, testMigrationSpec())
	if err != nil {
		t.Fatal(err)
	}
	source.Status = model.ReleaseStatusSucceeded
	st.UpdateRelease(source)
	current, err := service.CreateRelease(context.Background(), app.ID, 1, testMigrationSpec())
	if err != nil {
		t.Fatal(err)
	}
	current.Status = model.ReleaseStatusFailed
	st.UpdateRelease(current)
	_, spec, err := service.RollbackRelease(context.Background(), current.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Migration != nil {
		t.Fatal("rollback would rerun database migration")
	}
}

func TestMigrationJobOwnsTemporarySecretsAndBlocksUnfinishedJobs(t *testing.T) {
	spec := testMigrationSpec()
	app := ApplicationContext{ProjectName: "commerce", EnvironmentName: "prod", Namespace: "apps", ApplicationName: "orders", ReleaseSequence: 2}
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = types.UID("migration-owner")
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
		return false, nil, nil
	})
	adapter := NewKubernetesApplier(&k8sclient.Client{Clientset: client})
	if _, err := adapter.RunMigration(context.Background(), app, spec); err != nil {
		t.Fatal(err)
	}
	secrets, err := client.CoreV1().Secrets("apps").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(secrets.Items) != 1 {
		t.Fatalf("secrets=%+v err=%v", secrets, err)
	}
	if len(secrets.Items[0].OwnerReferences) != 1 || secrets.Items[0].OwnerReferences[0].UID != "migration-owner" {
		t.Fatal("temporary secret has no Job owner")
	}
	maps, _ := client.CoreV1().ConfigMaps("apps").List(context.Background(), metav1.ListOptions{})
	if len(maps.Items) != 1 || len(maps.Items[0].OwnerReferences) != 1 {
		t.Fatal("temporary config has no Job owner")
	}
	deployments, _ := client.AppsV1().Deployments("apps").List(context.Background(), metav1.ListOptions{})
	if len(deployments.Items) != 0 {
		t.Fatal("migration applied workload")
	}
	previous, _ := client.BatchV1().Jobs("apps").Get(context.Background(), migrationResourceName(app), metav1.GetOptions{})
	previous.Status.Conditions = nil
	client.BatchV1().Jobs("apps").UpdateStatus(context.Background(), previous, metav1.UpdateOptions{})
	app.ReleaseSequence++
	if _, err := adapter.RunMigration(context.Background(), app, spec); err == nil || !strings.Contains(err.Error(), "尚未结束") {
		t.Fatalf("unfinished job bypassed: %v", err)
	}
	jobs, _ := client.BatchV1().Jobs("apps").List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatal("created another Job while prior task runs")
	}
}
