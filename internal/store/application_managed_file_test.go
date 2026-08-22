package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestApplicationManagedFileUsesOptimisticVersion(t *testing.T) {
	s := setupTestDB(t)
	file := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "secret", ResourceName: "hysteria-secret", Key: "config.yaml", MountPath: "/etc/hysteria/config.yaml", Format: "text", CreatedBy: 1}
	if err := s.CreateApplicationManagedFile(file); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetApplicationManagedFile(7, file.ID)
	if err != nil || loaded.Version != 1 {
		t.Fatalf("file=%#v err=%v", loaded, err)
	}
	if _, err := s.AdvanceApplicationManagedFileVersion(7, file.ID, 2); err == nil {
		t.Fatal("expected stale version rejection")
	}
	version, err := s.AdvanceApplicationManagedFileVersion(7, file.ID, 1)
	if err != nil || version != 2 {
		t.Fatalf("version=%d err=%v", version, err)
	}
}

func TestDisableApplicationManagedFilesNotInDisablesRemovedBindings(t *testing.T) {
	s := setupTestDB(t)
	keep := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "configmap", ResourceName: "app-config", Key: "keep.yaml", MountPath: "/etc/app/keep.yaml", CreatedBy: 1}
	remove := &model.ApplicationManagedFile{ApplicationID: 7, ResourceKind: "secret", ResourceName: "app-secret", Key: "remove.yaml", MountPath: "/etc/app/remove.yaml", CreatedBy: 1}
	if err := s.CreateApplicationManagedFile(keep); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApplicationManagedFile(remove); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]struct{}{"configmap\x00app-config\x00keep.yaml": {}}
	if err := s.DisableApplicationManagedFilesNotIn(7, bindings); err != nil {
		t.Fatal(err)
	}
	files, err := s.ListApplicationManagedFiles(7)
	if err != nil || len(files) != 1 || files[0].ID != keep.ID {
		t.Fatalf("enabled files=%#v err=%v", files, err)
	}
}
