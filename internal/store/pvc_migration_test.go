package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestPersistentVolumeMigrationStoreFindsActiveSourceClaim(t *testing.T) {
	st := setupTestDB(t)
	migration := &model.PersistentVolumeMigration{EnvironmentID: 1, ApplicationID: 2, SourcePVCName: "karakeep-data", TargetPVCName: "karakeep-data-migration-1", SourceNodeName: "node-a", TargetNodeName: "node-b", Status: model.PVCMigrationStatusCopying}
	if err := st.CreatePersistentVolumeMigration(migration); err != nil {
		t.Fatal(err)
	}
	active, err := st.FindActivePVCMigration(1, "karakeep-data")
	if err != nil || active.ID != migration.ID {
		t.Fatalf("expected active migration, got %#v, %v", active, err)
	}
	if err := st.UpdatePersistentVolumeMigration(migration, model.PVCMigrationStatusSucceeded, "ready"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindActivePVCMigration(1, "karakeep-data"); err == nil {
		t.Fatal("terminal migration must not lock source claim")
	}
}

func TestHostDirectoryPVCImportStoreLocksClaimUntilTerminal(t *testing.T) {
	st := setupTestDB(t)
	task := &model.HostDirectoryPVCImport{EnvironmentID: 1, PVCName: "karakeep-data", SourceServerID: 2, SourcePath: "/srv/legacy", TargetNodeName: "node-a", TargetPath: "/var/lib/k3s/storage/pvc", BackupPath: "/srv/legacy/.cylism-import-backups/1.tar.gz", Status: model.PVCImportStatusCopying}
	if err := st.CreateHostDirectoryPVCImport(task); err != nil {
		t.Fatal(err)
	}
	active, err := st.FindActiveHostDirectoryPVCImport(1, "karakeep-data")
	if err != nil || active.ID != task.ID {
		t.Fatalf("expected active import, got %#v, %v", active, err)
	}
	if err := st.UpdateHostDirectoryPVCImport(task, model.PVCImportStatusSucceeded, "verified"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindActiveHostDirectoryPVCImport(1, "karakeep-data"); err == nil {
		t.Fatal("terminal import must not lock source claim")
	}
}
