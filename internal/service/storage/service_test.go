package storage

import (
	"errors"
	"strings"
	"testing"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
)

type fakeK8s struct {
	listed  bool
	deleted string
	created string
}

func (f *fakeK8s) ListPVCs(string) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	f.listed = true
	return []k8sclient.PersistentVolumeClaimInfo{{Name: "data"}}, nil
}
func (f *fakeK8s) ListManagedPVCs(ns string, id uint) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	return []k8sclient.PersistentVolumeClaimInfo{{Namespace: ns, EnvironmentID: id}}, nil
}
func (f *fakeK8s) GetManagedPVC(string, string, uint) (*k8sclient.PersistentVolumeClaimInfo, error) {
	return nil, nil
}
func (f *fakeK8s) CreateManagedPVC(namespace string, _ uint, request k8sclient.PersistentVolumeClaimRequest) (*corev1.PersistentVolumeClaim, error) {
	f.created = namespace + "/" + request.Name
	return &corev1.PersistentVolumeClaim{}, nil
}
func (f *fakeK8s) DeleteManagedPVC(ns, name string, _ uint) error {
	f.deleted = ns + "/" + name
	return nil
}
func (f *fakeK8s) ListStorageClasses() ([]k8sclient.StorageClassInfo, error) { return nil, nil }

type fakeEnv struct{ env *model.Environment }

func (f fakeEnv) GetEnvironmentByID(uint) (*model.Environment, error) {
	if f.env == nil {
		return nil, errors.New("missing")
	}
	return f.env, nil
}

func TestServiceResolvesEnvironmentNamespace(t *testing.T) {
	s := NewService(&fakeK8s{}, fakeEnv{env: &model.Environment{Namespace: "project-a"}})
	claims, err := s.ListPVCs("", 2)
	if err != nil || len(claims) != 1 || claims[0].Namespace != "project-a" {
		t.Fatalf("unexpected claims: %#v, %v", claims, err)
	}
}

func TestServiceRejectsNamespaceMismatch(t *testing.T) {
	s := NewService(&fakeK8s{}, fakeEnv{env: &model.Environment{Namespace: "project-a"}})
	if _, err := s.ListPVCs("other", 2); err == nil {
		t.Fatal("expected namespace mismatch")
	}
}

func TestServiceDeletesNamedPVC(t *testing.T) {
	f := &fakeK8s{}
	if err := NewService(f, nil).DeletePVC("ns", "data", 1); err != nil || f.deleted != "ns/data" {
		t.Fatalf("delete failed: %v %q", err, f.deleted)
	}
}

func TestServiceGetsPVCThroughAdapter(t *testing.T) {
	f := &fakeK8s{}
	s := NewService(f, nil)
	if _, err := s.GetPVC("ns", "data", 0); err != nil {
		t.Fatal(err)
	}
}

func TestServiceCreatesPVCInEnvironmentNamespace(t *testing.T) {
	f := &fakeK8s{}
	s := NewService(f, fakeEnv{env: &model.Environment{Namespace: "project-a"}})
	if _, err := s.CreatePVC("", 1, k8sclient.PersistentVolumeClaimRequest{Name: "data", Storage: "1Gi"}); err != nil {
		t.Fatal(err)
	}
	if f.created != "project-a/data" {
		t.Fatalf("created PVC = %q", f.created)
	}
}

func TestValidatePVCDeletionProtectsActiveAndReferencedClaims(t *testing.T) {
	s := NewService(nil, nil)
	if err := s.ValidatePVCDeletion(true, true, nil, "Delete"); err == nil {
		t.Fatal("expected active migration to block deletion")
	}
	if err := s.ValidatePVCDeletion(true, false, []string{"工作负载：api"}, "Delete"); err == nil {
		t.Fatal("expected references to block deletion")
	}
}

func TestValidatePVCDeletionRequiresExplicitDataConfirmation(t *testing.T) {
	s := NewService(nil, nil)
	err := s.ValidatePVCDeletion(false, false, nil, "Retain")
	if err == nil || !strings.Contains(err.Error(), "Retain") {
		t.Fatalf("expected reclaim policy in confirmation error, got %v", err)
	}
	if err := s.ValidatePVCDeletion(true, false, nil, "Retain"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateHostDirectoryImportPath(t *testing.T) {
	cleaned, ok := ValidateHostDirectoryImportPath(" /srv/data/../payload ")
	if !ok || cleaned != "/srv/payload" {
		t.Fatalf("unexpected normalized path %q, %v", cleaned, ok)
	}
	if _, ok := ValidateHostDirectoryImportPath("/etc"); ok {
		t.Fatal("expected protected path to be rejected")
	}
	if !HostDirectoryImportPathsOverlap("/srv/data", "/srv/data/nested") {
		t.Fatal("expected nested paths to overlap")
	}
}

func TestValidateBackupRoot(t *testing.T) {
	root, err := ValidateBackupRoot("/backup/../archives")
	if err != nil || root != "/archives" {
		t.Fatalf("unexpected backup root %q, %v", root, err)
	}
	if _, err := ValidateBackupRoot("/"); err == nil {
		t.Fatal("expected root directory to be rejected")
	}
}

func TestServiceOwnsPersistentVolumeTaskRecords(t *testing.T) {
	records, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(nil, nil, records)
	migration := &model.PersistentVolumeMigration{EnvironmentID: 1, SourcePVCName: "data", Status: model.PVCMigrationStatusPending}
	if err := s.CreateMigration(migration); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMigration(migration, model.PVCMigrationStatusPreflight, "checking"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMigration(migration.ID)
	if err != nil || got.Status != model.PVCMigrationStatusPreflight {
		t.Fatalf("unexpected migration: %#v, %v", got, err)
	}
}

func TestServiceDispatchesLongRunningStorageTasks(t *testing.T) {
	started := make(chan string, 4)
	s := NewService(nil, nil).WithAsyncExecutor(AsyncExecutor{
		RunMigration: func(uint, string) { started <- "migration" },
		RunImport:    func(uint, bool) { started <- "import" },
		RunBackup:    func(uint) { started <- "backup" },
		RunRestore:   func(uint) { started <- "restore" },
	})
	if err := s.StartMigration(1, "helper"); err != nil {
		t.Fatal(err)
	}
	if err := s.StartImport(2, true); err != nil {
		t.Fatal(err)
	}
	if err := s.StartBackup(3); err != nil {
		t.Fatal(err)
	}
	if err := s.StartRestore(4); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 4 {
		seen[<-started] = true
	}
	for _, name := range []string{"migration", "import", "backup", "restore"} {
		if !seen[name] {
			t.Fatalf("executor %s was not dispatched", name)
		}
	}
}

func TestServiceRejectsMissingStorageExecutor(t *testing.T) {
	if err := NewService(nil, nil).StartBackup(1); err == nil {
		t.Fatal("expected missing executor error")
	}
}
