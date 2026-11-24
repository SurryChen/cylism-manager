package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

type testRepository struct {
	mu         sync.Mutex
	statuses   []string
	failStatus string
	item       model.ObservabilityMigration
}

func (r *testRepository) ListServers() ([]model.Server, error) {
	id := uint(2)
	return []model.Server{{Name: "source", K8sNodeName: "a", ClusterID: &id}, {Name: "target", K8sNodeName: "b", ClusterID: &id}}, nil
}
func (r *testRepository) CreateObservabilityMigration(i *model.ObservabilityMigration) error {
	i.ID = 1
	i.TaskID = 10
	r.item = *i
	return nil
}
func (r *testRepository) SaveObservabilityMigration(i *model.ObservabilityMigration, _ string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i.Status == r.failStatus {
		return errors.New("database unavailable")
	}
	control := r.item.Control
	r.item = *i
	r.item.Control = control
	r.statuses = append(r.statuses, i.Status)
	return nil
}
func (r *testRepository) GetObservabilityMigration(id uint) (*model.ObservabilityMigration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.item.ID != id {
		return nil, errors.New("not found")
	}
	item := r.item
	return &item, nil
}
func (r *testRepository) SetObservabilityMigrationControl(_ uint, control, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.item.Control = control
	return nil
}
func (r *testRepository) UpdateObservabilityMigrationProgress(_ uint, total, copied, rate int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.item.BytesTotal, r.item.BytesCopied, r.item.BytesPerSecond = total, copied, rate
	return nil
}
func (r *testRepository) ListObservabilityMigrations(uint, string) ([]model.ObservabilityMigration, error) {
	return []model.ObservabilityMigration{r.item}, nil
}
func (r *testRepository) ActiveObservabilityMigrations() ([]model.ObservabilityMigration, error) {
	return []model.ObservabilityMigration{r.item}, nil
}

type testAdapter struct {
	snapshot                 *k8s.ComponentStorageSnapshot
	calls                    []string
	targetFails, unlockFails bool
}

func (a *testAdapter) ComponentStorageSnapshot(context.Context, string) (*k8s.ComponentStorageSnapshot, error) {
	copy := *a.snapshot
	return &copy, nil
}
func (*testAdapter) ValidateComponentMigrationNode(context.Context, string) error { return nil }
func (*testAdapter) LockComponentMigration(context.Context, string, uint) error   { return nil }
func (a *testAdapter) UnlockComponentMigration(context.Context, string, uint) error {
	if a.unlockFails {
		return errors.New("unlock unavailable")
	}
	a.calls = append(a.calls, "unlock")
	return nil
}
func (a *testAdapter) ProvisionComponentMigration(context.Context, *k8s.ComponentStorageSnapshot, string, string, uint) (*k8s.PersistentVolumeClaimInfo, error) {
	a.calls = append(a.calls, "provision")
	return &k8s.PersistentVolumeClaimInfo{LocalPath: "/target"}, nil
}
func (a *testAdapter) SetComponentStorage(_ context.Context, _ *k8s.ComponentStorageSnapshot, pvc, node string, replicas int32, restore bool) error {
	a.calls = append(a.calls, fmt.Sprintf("set:%s:%s:%d:%t", pvc, node, replicas, restore))
	return nil
}
func (a *testAdapter) WaitComponentStorage(_ context.Context, _ *k8s.ComponentStorageSnapshot, node, _ string, ready bool) error {
	a.calls = append(a.calls, fmt.Sprintf("wait:%s:%t", node, ready))
	if ready && node == "b" && a.targetFails {
		return errors.New("target not ready")
	}
	return nil
}
func (a *testAdapter) DeletePVCBindingPodContext(context.Context, string, string) error {
	a.calls = append(a.calls, "delete-binder")
	return nil
}

type testTransfer struct {
	failExec, failCopy, badHash bool
	protectedStorageParent      bool
	calls                       []string
	onCopy                      func(context.Context)
}

func (t *testTransfer) Exec(_ context.Context, s *model.Server, command string) ([]byte, error) {
	t.calls = append(t.calls, command)
	if t.failExec {
		return nil, errors.New("ssh unavailable")
	}
	if strings.Contains(command, "du -sb") {
		return []byte("1000 /source"), nil
	}
	if strings.Contains(command, "df -B1") {
		if t.protectedStorageParent && !strings.Contains(command, "sudo -n df -B1") {
			return []byte("df: /target: Permission denied"), errors.New("Process exited with status 1")
		}
		return []byte("Avail\n9999999999"), nil
	}
	if strings.Contains(command, "tar --sort") {
		v := "a"
		if t.badHash && s.Name == "target" {
			v = "b"
		}
		return []byte(strings.Repeat(v, 64) + "  -\n"), nil
	}
	return nil, nil
}
func TestMigrationChecksDiskUnderProtectedStorageParent(t *testing.T) {
	s, _, _, tr, item := fixture(k8s.InfrastructureVictoriaMetrics)
	tr.protectedStorageParent = true
	s.run(item)
	if item.Status != "succeeded" {
		t.Fatalf("root-only storage parent prevented migration: %s", item.Detail)
	}
}
func (t *testTransfer) Copy(ctx context.Context, _ *model.Server, _ *model.Server, _, _ string, _ CopyControl) (int64, error) {
	t.calls = append(t.calls, "copy")
	if t.onCopy != nil {
		t.onCopy(ctx)
	}
	if t.failCopy {
		return 20, errors.New("stream failed")
	}
	return 1024, nil
}
func fixture(component string) (*Service, *testRepository, *testAdapter, *testTransfer, *model.ObservabilityMigration) {
	r := &testRepository{}
	a := &testAdapter{snapshot: &k8s.ComponentStorageSnapshot{Component: component, UID: "uid", PVC: "source", Node: "a", Path: "/source"}}
	tr := &testTransfer{}
	s := New(r, func(context.Context, uint) (Adapter, error) { return a, nil }, tr)
	i := &model.ObservabilityMigration{ID: 1, TaskID: 10, ClusterID: 2, Component: component, SourceNode: "a", TargetNode: "b", SourcePVC: "source", Status: "pending", Snapshot: k8s.EncodeComponentStorageSnapshot(a.snapshot)}
	return s, r, a, tr, i
}
func TestMigrationCopiesVerifiesThenCutsOver(t *testing.T) {
	for _, component := range []string{k8s.InfrastructureVictoriaMetrics, k8s.InfrastructureLoki} {
		t.Run(component, func(t *testing.T) {
			s, r, a, tr, i := fixture(component)
			s.run(i)
			if i.Status != "succeeded" {
				t.Fatalf("status=%s: %s", i.Status, i.Detail)
			}
			want := "preflight,provisioning_target,stopping_source,copying,verifying,cutover,waiting_ready,finalizing,succeeded"
			if strings.Join(r.statuses, ",") != want {
				t.Fatalf("stages %v", r.statuses)
			}
			calls := strings.Join(a.calls, ",")
			if !strings.Contains(calls, "set:source:a:0:false,wait:a:false") || !strings.Contains(calls, ":b:1:false,wait:b:true,unlock") {
				t.Fatal(calls)
			}
			if !strings.Contains(strings.Join(tr.calls, ","), "tar --sort=name --numeric-owner") {
				t.Fatal("missing complete directory verification")
			}
			if i.BytesCopied != 1024 {
				t.Fatal(i.BytesCopied)
			}
		})
	}
}

func TestMigrationDoesNotImposeCopyDeadline(t *testing.T) {
	for _, component := range []string{k8s.InfrastructureVictoriaMetrics, k8s.InfrastructureLoki} {
		t.Run(component, func(t *testing.T) {
			s, _, _, tr, item := fixture(component)
			copied := false
			tr.onCopy = func(ctx context.Context) {
				copied = true
				if deadline, ok := ctx.Deadline(); ok {
					t.Fatalf("slow copy would be interrupted by task deadline %v", deadline)
				}
				if err := ctx.Err(); err != nil {
					t.Fatalf("copy started with canceled context: %v", err)
				}
			}
			s.run(item)
			if !copied || item.Status != "succeeded" {
				t.Fatalf("copy did not finish migration: copied=%t status=%s detail=%s", copied, item.Status, item.Detail)
			}
		})
	}
}
func TestMigrationFailuresRestoreOnlyAfterStopping(t *testing.T) {
	for _, scenario := range []string{"ssh", "copy", "checksum", "startup"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, a, tr, i := fixture(k8s.InfrastructureLoki)
			tr.failExec = scenario == "ssh"
			tr.failCopy = scenario == "copy"
			tr.badHash = scenario == "checksum"
			a.targetFails = scenario == "startup"
			s.run(i)
			calls := strings.Join(a.calls, ",")
			if scenario == "ssh" {
				if i.Status != "failed" || strings.Contains(calls, "set:") {
					t.Fatalf("source touched: %s %s", i.Status, calls)
				}
				return
			}
			if i.Status != "rolled_back" || !strings.Contains(calls, "set:source:a:1:true,wait:a:true") {
				t.Fatalf("rollback missing: %s %s", i.Status, calls)
			}
			if scenario != "startup" && strings.Contains(calls, ":b:1:false") {
				t.Fatalf("unverified target started: %s", calls)
			}
		})
	}
}
func TestRecoveryDoesNotReplayPartialCopy(t *testing.T) {
	for _, stage := range []string{"preflight", "provisioning_target", "copying", "waiting_ready", "finalizing"} {
		t.Run(stage, func(t *testing.T) {
			s, _, a, tr, i := fixture(k8s.InfrastructureVictoriaMetrics)
			i.Status = stage
			i.TargetPVC = "target"
			s.run(i)
			if len(tr.calls) != 0 {
				t.Fatal("recovery copied data again")
			}
			calls := strings.Join(a.calls, ",")
			switch stage {
			case "preflight", "provisioning_target":
				if i.Status != "failed" || strings.Contains(calls, "set:") {
					t.Fatal(i.Status, calls)
				}
			case "copying":
				if i.Status != "rolled_back" {
					t.Fatal(i.Status)
				}
			default:
				if i.Status != "succeeded" || strings.Contains(calls, "set:") {
					t.Fatal(i.Status, calls)
				}
			}
		})
	}
}
func TestHealthyTargetMetadataFailureNeverRollsBack(t *testing.T) {
	for _, failure := range []string{"finalizing", "succeeded", "unlock"} {
		t.Run(failure, func(t *testing.T) {
			s, r, a, _, i := fixture(k8s.InfrastructureLoki)
			r.failStatus = failure
			a.unlockFails = failure == "unlock"
			s.run(i)
			if strings.Contains(strings.Join(a.calls, ","), ":true") { // wait calls also contain true; inspect restore writes only.
				for _, call := range a.calls {
					if strings.HasPrefix(call, "set:") && strings.HasSuffix(call, ":true") {
						t.Fatalf("healthy target rolled back: %v", a.calls)
					}
				}
			}
			if i.Status != "waiting_ready" && i.Status != "finalizing" {
				t.Fatalf("lost recovery checkpoint: %s", i.Status)
			}
			r.failStatus = ""
			a.unlockFails = false
			s.run(i)
			if i.Status != "succeeded" {
				t.Fatal(i.Status)
			}
		})
	}
}
func TestStartRejectsSameNodeAndWrongCluster(t *testing.T) {
	s, _, _, _, _ := fixture(k8s.InfrastructureLoki)
	if _, err := s.Start(t.Context(), 2, k8s.InfrastructureLoki, "a"); err == nil {
		t.Fatal("same node accepted")
	}
	if _, err := s.Start(t.Context(), 3, k8s.InfrastructureLoki, "b"); err == nil {
		t.Fatal("cross cluster accepted")
	}
}

type controlledTestTransfer struct {
	*testTransfer
	started chan struct{}
	finish  chan struct{}
	copied  atomic.Int64
}

func (tr *controlledTestTransfer) Copy(ctx context.Context, _, _ *model.Server, _, _ string, control CopyControl) (int64, error) {
	close(tr.started)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := control.Wait(ctx); err != nil {
			return tr.copied.Load(), err
		}
		select {
		case <-ctx.Done():
			return tr.copied.Load(), context.Cause(ctx)
		case <-tr.finish:
			return tr.copied.Load(), nil
		case <-ticker.C:
			control.Progress(tr.copied.Add(10))
		}
	}
}

func awaitMigration(t *testing.T, repo *testRepository, predicate func(*model.ObservabilityMigration) bool) *model.ObservabilityMigration {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		item, err := repo.GetObservabilityMigration(1)
		if err == nil && predicate(item) {
			return item
		}
		time.Sleep(5 * time.Millisecond)
	}
	item, _ := repo.GetObservabilityMigration(1)
	t.Fatalf("migration state did not arrive: %+v", item)
	return nil
}

func TestMigrationPauseResumeAndCancel(t *testing.T) {
	for _, action := range []string{"resume", "cancel"} {
		t.Run(action, func(t *testing.T) {
			s, repo, adapter, base, item := fixture(k8s.InfrastructureLoki)
			tr := &controlledTestTransfer{testTransfer: base, started: make(chan struct{}), finish: make(chan struct{})}
			s.transfer = tr
			s.launch(*item)
			select {
			case <-tr.started:
			case <-time.After(5 * time.Second):
				t.Fatal("copy did not start")
			}
			t.Cleanup(func() {
				s.mu.Lock()
				run := s.running[1]
				s.mu.Unlock()
				if run != nil {
					run.cancel(context.Canceled)
				}
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					s.mu.Lock()
					alive := s.running[1] != nil
					s.mu.Unlock()
					if !alive {
						return
					}
					time.Sleep(time.Millisecond)
				}
				t.Error("worker did not stop")
			})
			awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.BytesCopied > 0 && i.BytesPerSecond > 0 })
			if _, err := s.Control(2, k8s.InfrastructureLoki, 1, "pause"); err != nil {
				t.Fatal(err)
			}
			paused := awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "paused" })
			before := tr.copied.Load()
			time.Sleep(50 * time.Millisecond)
			if got := tr.copied.Load(); got != before {
				t.Fatalf("paused transfer moved: %d -> %d", before, got)
			}
			if paused.BytesCopied == 0 || paused.Control != "pause" {
				t.Fatalf("pause lost progress: %+v", paused)
			}
			if _, err := s.Control(2, k8s.InfrastructureLoki, 1, action); err != nil {
				t.Fatal(err)
			}
			if action == "resume" {
				awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "copying" && i.Control == "" })
				close(tr.finish)
				result := awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "succeeded" })
				if result.BytesCopied < before {
					t.Fatal("resume reset transfer bytes")
				}
			} else {
				awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "rolled_back" })
				// Check calls only after worker exit; they are owned by the worker.
				s.mu.Lock()
				run := s.running[1]
				s.mu.Unlock()
				if run != nil {
					<-run.ctx.Done()
				}
				if !strings.Contains(strings.Join(adapter.calls, ","), "set:source:a:1:true") {
					t.Fatal("cancel did not restore source")
				}
			}
		})
	}
}

func TestPausedRecoveryWaitsForUserAndRejectsUnsafeResume(t *testing.T) {
	s, repo, adapter, _, item := fixture(k8s.InfrastructureLoki)
	item.Status, item.Control = "paused", "pause"
	repo.item = *item
	s.Recover(context.Background())
	if len(s.running) != 0 || len(adapter.calls) != 0 {
		t.Fatal("recovery changed a user-paused migration")
	}
	if _, err := s.Control(2, k8s.InfrastructureLoki, 1, "resume"); err == nil {
		t.Fatal("lost SSH stream was resumed")
	}
	if _, err := s.Control(3, k8s.InfrastructureLoki, 1, "cancel"); err == nil {
		t.Fatal("cross-cluster control accepted")
	}
	if _, err := s.Control(2, k8s.InfrastructureVictoriaMetrics, 1, "cancel"); err == nil {
		t.Fatal("wrong component control accepted")
	}
	if _, err := s.Control(2, k8s.InfrastructureLoki, 1, "cancel"); err != nil {
		t.Fatal(err)
	}
	awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "rolled_back" })
}

func TestCancelCannotCrossCutoverBoundary(t *testing.T) {
	s, repo, _, _, item := fixture(k8s.InfrastructureLoki)
	run := newMigrationRun("")
	defer run.cancel(nil)
	s.running[1] = run
	item.Status = "verifying"
	repo.item = *item
	if _, err := s.Control(2, k8s.InfrastructureLoki, 1, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err := s.checkpoint(item, "cutover", "cutover", false); err == nil {
		t.Fatal("cutover won after cancellation was accepted")
	}
	item.Status, repo.item.Status, repo.item.Control = "waiting_ready", "waiting_ready", ""
	if _, err := s.Control(2, k8s.InfrastructureLoki, 1, "cancel"); err == nil {
		t.Fatal("cancel accepted after enabling target writes")
	}
}

func TestPauseAtCopyCompletionBlocksVerification(t *testing.T) {
	s, repo, _, _, item := fixture(k8s.InfrastructureLoki)
	run := newMigrationRun("")
	defer run.cancel(nil)
	s.running[item.ID] = run
	item.Status = "copying"
	repo.item = *item
	run.onPause = func(paused bool) error {
		status := "copying"
		if paused {
			status = "paused"
		}
		return s.checkpoint(item, status, status, false)
	}
	if _, err := s.Control(2, k8s.InfrastructureLoki, item.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.checkpoint(item, "verifying", "verifying", false) }()
	t.Cleanup(func() { run.cancel(context.Canceled) })
	awaitMigration(t, repo, func(i *model.ObservabilityMigration) bool { return i.Status == "paused" })
	select {
	case err := <-done:
		t.Fatalf("verification bypassed pause: %v", err)
	default:
	}
	if _, err := s.Control(2, k8s.InfrastructureLoki, item.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verification did not resume")
	}
	if item.Status != "verifying" {
		t.Fatal(item.Status)
	}
}
