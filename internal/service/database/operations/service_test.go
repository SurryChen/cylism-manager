package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testService(t *testing.T) (*Service, *k8s.Client) {
	t.Helper()
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", Labels: map[string]string{corev1.LabelHostname: "worker"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}},
	)}
	service := New(s, bytes.Repeat([]byte("s"), 32), func(context.Context, uint) (*k8s.Client, error) { return client, nil })
	return service, client
}

func mustCreate(t *testing.T, s *Service, value any) {
	t.Helper()
	if err := s.store.DB().Create(value).Error; err != nil {
		t.Fatal(err)
	}
}

func seedTask(t *testing.T, s *Service, kind string, phase int) *model.DatabaseOperation {
	t.Helper()
	req := Request{Kind: kind, Source: "instance:1", Target: "instance:2", ClusterID: 1, NodeName: "worker", Image: DefaultImage, StorageSize: "1Gi", StorageClass: "local"}
	raw, _ := json.Marshal(req)
	task := &model.DatabaseOperation{Kind: kind, Source: req.Source, Target: req.Target, ClusterID: 1, NodeName: "worker", Spec: string(raw), EncryptedInput: "encrypted", Status: "queued", Phase: phase, PhaseName: Phases(kind)[phase]}
	mustCreate(t, s, task)
	mustCreate(t, s, &model.DatabaseOperationLock{Resource: task.Target, TaskID: task.ID})
	return task
}

func readTask(t *testing.T, s *Service, id uint) model.DatabaseOperation {
	t.Helper()
	var task model.DatabaseOperation
	if err := s.store.DB().First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	return task
}

func TestPauseAtCheckpointAndRestartRecovery(t *testing.T) {
	s, _ := testService(t)
	task := seedTask(t, s, "migrate", 0)
	calls := 0
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) {
		calls++
		return true, "", nil
	}
	if err := s.Control(task.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	s.Reconcile(context.Background())
	paused := readTask(t, s, task.ID)
	if paused.Status != "paused" || calls != 0 {
		t.Fatal("queued pause submitted a mutation")
	}
	if err := AssertUnlocked(s.store.DB(), task.Target); err == nil {
		t.Fatal("paused task lost target lock")
	}
	if err := s.Control(task.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	s.execute = func(ctx context.Context, _ *model.DatabaseOperation, _ Request) (bool, string, error) {
		if err := s.Control(task.ID, "pause"); err != nil {
			t.Fatal(err)
		}
		return true, "", nil
	}
	s.Reconcile(context.Background())
	paused = readTask(t, s, task.ID)
	if paused.Status != "paused" || paused.Phase != 1 {
		t.Fatalf("pause during running phase lost: %+v", paused)
	}
	// A new process reads the checkpoint and never repeats the completed phase.
	restarted := New(s.store, s.key, s.client)
	restarted.execute = func(_ context.Context, current *model.DatabaseOperation, _ Request) (bool, string, error) {
		if current.Phase != 1 {
			t.Fatalf("replayed phase: %d", current.Phase)
		}
		return true, "", nil
	}
	if err := restarted.Control(task.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	restarted.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.Phase != 2 || got.Status != "queued" {
		t.Fatalf("recovery lost checkpoint: %+v", got)
	}
}

func TestFailureRetainsLockAndCancelWaitsForResources(t *testing.T) {
	s, _ := testService(t)
	task := seedTask(t, s, "migrate", 2)
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) {
		return false, "", errors.New("import failed")
	}
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.Status != "failed" {
		t.Fatal(got.Status)
	}
	if err := AssertUnlocked(s.store.DB(), task.Target); err == nil {
		t.Fatal("partial import unlocked target")
	}
	if err := s.Control(task.ID, "resume"); err == nil {
		t.Fatal("failed mutations must not be blindly retried")
	}
	if err := s.Control(task.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	s.cleanup = func(context.Context, *model.DatabaseOperation) error { return errors.New("pod terminating") }
	s.Reconcile(context.Background())
	if err := AssertUnlocked(s.store.DB(), task.Target); err == nil {
		t.Fatal("unlocked before Pod exited")
	}
	s.cleanup = func(context.Context, *model.DatabaseOperation) error { return nil }
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.Status != "cancelled" || got.EncryptedInput != "" {
		t.Fatal("cancel did not clear encrypted input")
	}
	if err := AssertUnlocked(s.store.DB(), task.Target); err != nil {
		t.Fatal(err)
	}
}

func TestCancelDuringPhaseDoesNotAdvanceOrLoseReplicationAudit(t *testing.T) {
	s, _ := testService(t)
	task := seedTask(t, s, "replicate", 3)
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) {
		if err := s.Control(task.ID, "cancel"); err != nil {
			t.Fatal(err)
		}
		return true, "", nil
	}
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.Status != "cancelling" || got.Phase != 3 {
		t.Fatal("late phase overwrote cancel")
	}
	audits := 0
	s.settle = func(context.Context, *model.DatabaseOperation) (bool, error) { audits++; return true, nil }
	s.cleanup = func(context.Context, *model.DatabaseOperation) error {
		return errors.New("waiting for audit Pod deletion")
	}
	s.Reconcile(context.Background())
	s.Reconcile(context.Background())
	if audits != 1 {
		t.Fatalf("cancellation audit replayed %d times", audits)
	}
	s.cleanup = func(context.Context, *model.DatabaseOperation) error { return nil }
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.Status != "cancelled" {
		t.Fatal(got.Status)
	}
}

func TestSuccessfulTaskRetriesCredentialCleanup(t *testing.T) {
	s, _ := testService(t)
	task := seedTask(t, s, "inspect", 0)
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) {
		return true, `{"schemas":["app"]}`, nil
	}
	s.cleanup = func(context.Context, *model.DatabaseOperation) error { return errors.New("temporary network failure") }
	s.Reconcile(context.Background())
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); !got.CleanupPending || got.EncryptedInput != "" || got.Status != "succeeded" {
		t.Fatal("cleanup failure lost reconciliation")
	}
	s.cleanup = func(context.Context, *model.DatabaseOperation) error { return nil }
	s.Reconcile(context.Background())
	if got := readTask(t, s, task.ID); got.CleanupPending {
		t.Fatal("cleanup never completed")
	}
}

func TestQueueEncryptsCredentialsAndLocksConflictingOperations(t *testing.T) {
	s, _ := testService(t)
	admin, _ := security.Encrypt(s.key, "admin-password")
	root, _ := security.Encrypt(s.key, "root-password")
	source := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306, AdminUsername: "administrator", EncryptedAdmin: admin}
	target := model.DatabaseInstance{Name: "standalone", Mode: "managed", ClusterID: 1, Host: "mysql.internal", Port: 3306, EncryptedRoot: root}
	mustCreate(t, s, &source)
	mustCreate(t, s, &target)
	req := Request{Kind: "replicate", Source: Reference(source.ID), Target: Reference(target.ID), ClusterID: 1, NodeName: "worker", ReplicationUser: "repl", ReplicationPassword: "replication-password", Connections: []SwitchConnection{{Password: "irrelevant-password"}}}
	task, err := s.Queue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(task)
	for _, secret := range []string{"admin-password", "root-password", "replication-password", "irrelevant-password"} {
		if strings.Contains(task.Spec, secret) || strings.Contains(string(serialized), secret) {
			t.Fatal("credential in task spec or API")
		}
	}
	plain, err := security.Decrypt(s.key, task.EncryptedInput)
	if err != nil || !strings.Contains(plain, "replication-password") {
		t.Fatal("credentials not recoverable from encrypted payload")
	}
	if _, err := s.Queue(context.Background(), req); err == nil {
		t.Fatal("conflicting task accepted")
	}
	var count int64
	s.store.DB().Model(&model.DatabaseOperation{}).Count(&count)
	if count != 1 {
		t.Fatal("conflict left an orphan task")
	}
	s.store.DB().Model(&model.DatabaseReplication{}).Count(&count)
	if count != 0 {
		t.Fatal("created a planned replication relation")
	}
}

func TestReplicationRelationIsRecordedAtCreatedChannelCheckpoint(t *testing.T) {
	s, _ := testService(t)
	task := seedTask(t, s, "replicate", 3)
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) { return true, "", nil }
	s.Reconcile(context.Background())
	var relation model.DatabaseReplication
	if err := s.store.DB().Where("target = ?", task.Target).First(&relation).Error; err != nil {
		t.Fatal(err)
	}
	if relation.Status != "checking" || relation.Source != task.Source {
		t.Fatalf("incorrect relation: %+v", relation)
	}
	s.Reconcile(context.Background())
	s.store.DB().First(&relation, relation.ID)
	if relation.Status != "running" {
		t.Fatal("verified relation not marked running")
	}
}

func TestJobHasDurableStorageAndNoAutomaticMutationReplay(t *testing.T) {
	task := &model.DatabaseOperation{ID: 17, Kind: "migrate", Phase: 2, NodeName: "worker"}
	job := buildJob(task, Request{Image: DefaultImage})
	if job.Spec.ActiveDeadlineSeconds != nil || job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Fatal("Job has execution deadline or automatic retries")
	}
	if job.Spec.Template.Spec.Volumes[1].PersistentVolumeClaim.ClaimName != taskName(17) || job.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "worker" {
		t.Fatal("phase did not reuse snapshot volume or execution node")
	}
	if *job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("SQL job has unnecessary Kubernetes credentials")
	}
}

func TestRawSQLAndPasswordsNeverBecomeProgressOrErrors(t *testing.T) {
	output := []byte("mysql://root:password@host\nCYLISM_EVENT:{\"error\":\"mysql://secret\"}\nCYLISM_EVENT:{\"progress_bytes\":1048576}\nCYLISM_EVENT:{\"done\":true}\n")
	progress, result, reason, done := parseEvents(output)
	if !done || result != "" || !strings.Contains(progress, "1.0 MiB") {
		t.Fatal("valid event stream was not read")
	}
	if strings.Contains(errorMessage(reason), "secret") || strings.Contains(progress, "password") {
		t.Fatal("raw connector error escaped allowlist")
	}
}
