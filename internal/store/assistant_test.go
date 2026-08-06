package store

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAssistantRuntimeMigrationStoreFindsActiveMigration(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	migration := &model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "default", SourcePVCName: "cylism-ops-agent-audit", TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit", TargetNodeName: "worker-a", Storage: "1Gi", Status: model.AssistantRuntimeMigrationCopying}
	if err := store.CreateAssistantRuntimeMigration(migration); err != nil {
		t.Fatal(err)
	}
	active, err := store.FindActiveAssistantRuntimeMigration()
	if err != nil || active.ID != migration.ID {
		t.Fatalf("active migration = %#v, %v", active, err)
	}
	if err := store.UpdateAssistantRuntimeMigration(migration, model.AssistantRuntimeMigrationSucceeded, "complete"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindActiveAssistantRuntimeMigration(); err == nil {
		t.Fatal("completed migration must not be active")
	}
}
