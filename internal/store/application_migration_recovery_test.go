package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"testing"
)

func TestRecoverInterruptedMigrationReleaseRequiresExplicitRetry(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	migration := &model.Release{ApplicationID: 1, Sequence: 1, Image: "orders:1", DesiredSpec: "{\"migration\":{\"enabled\":true,\"target\":\"1\"}}", Status: model.ReleaseStatusMigrating, CreatedBy: 1}
	regular := &model.Release{ApplicationID: 2, Sequence: 1, Image: "web:1", DesiredSpec: "{}", Status: model.ReleaseStatusApplying, CreatedBy: 1}
	st.CreateRelease(migration)
	st.CreateRelease(regular)
	st.CreateReleaseOperation(&model.ReleaseOperation{ReleaseID: migration.ID, Step: "database_migration", Status: model.ReleaseOperationRunning})
	if err = st.RecoverInterruptedMigrationReleases(); err != nil {
		t.Fatal(err)
	}
	recovered, _ := st.GetRelease(migration.ID)
	if recovered.Status != model.ReleaseStatusFailed || recovered.CompletedAt == nil {
		t.Fatal("migration stuck in progress after restart")
	}
	for _, op := range recovered.Operations {
		if op.Status == model.ReleaseOperationRunning {
			t.Fatal("operation stuck running")
		}
	}
	untouched, _ := st.GetRelease(regular.ID)
	if untouched.Status != model.ReleaseStatusApplying {
		t.Fatal("unrelated release changed")
	}
	if err = st.RecoverInterruptedMigrationReleases(); err != nil {
		t.Fatal(err)
	}
	again, _ := st.GetRelease(migration.ID)
	if len(again.Operations) != len(recovered.Operations) {
		t.Fatal("recovery duplicated interruption")
	}
}
