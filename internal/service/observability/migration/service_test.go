package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
)

type testRepository struct {
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
	if i.Status == r.failStatus {
		return errors.New("database unavailable")
	}
	r.item = *i
	r.statuses = append(r.statuses, i.Status)
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
	calls                       []string
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
func (t *testTransfer) Copy(context.Context, *model.Server, *model.Server, string, string) (int64, error) {
	t.calls = append(t.calls, "copy")
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
