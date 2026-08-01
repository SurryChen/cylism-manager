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
