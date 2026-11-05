package store

import (
	"errors"
	"github.com/cylism/cylism-manager/internal/model"
	"testing"
)

func TestObservabilityMigrationPersistsTaskAndSurvivesRecovery(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "observability", Distribution: "k3s"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "target", Host: "10.0.0.1", ClusterID: &cluster.ID, K8sNodeName: "node-b"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	item := model.ObservabilityMigration{ClusterID: cluster.ID, Component: "loki", TargetNode: "node-b", Status: "pending", Detail: "created"}
	if err := st.CreateObservabilityMigration(&item); err != nil {
		t.Fatal(err)
	}
	if err := st.SetObservabilityMigrationControl(item.ID, "pause", "pause requested"); err != nil {
		t.Fatal(err)
	}
	item.Status = "paused"
	if err := st.SaveObservabilityMigration(&item, "paused", false); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateObservabilityMigrationProgress(item.ID, 2000, 1000, 250); err != nil {
		t.Fatal(err)
	}
	var checkpoint model.ObservabilityMigration
	if err := st.DB().First(&checkpoint, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if checkpoint.Control != "pause" || checkpoint.BytesTotal != 2000 || checkpoint.BytesCopied != 1000 || checkpoint.BytesPerSecond != 250 || checkpoint.ProgressAt == nil {
		t.Fatalf("control/progress checkpoint not persisted: %+v", checkpoint)
	}
	duplicate := item
	duplicate.ID = 0
	duplicate.TaskID = 0
	if err := st.CreateObservabilityMigration(&duplicate); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatal("missing cluster guard", err)
	}
	if err := st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	var task model.ClusterDeployment
	st.DB().First(&task, item.TaskID)
	if task.Status != "running" || task.Operation != "migrate-loki" {
		t.Fatalf("migration marked interrupted: %+v", task)
	}
	active, err := st.ActiveObservabilityMigrations()
	if err != nil || len(active) != 1 {
		t.Fatal(active, err)
	}
	item.Status = "succeeded"
	if err := st.SaveObservabilityMigration(&item, "ready", true); err != nil {
		t.Fatal(err)
	}
	st.DB().First(&task, item.TaskID)
	if task.Status != "success" {
		t.Fatal(task.Status)
	}
	logs, err := st.ListClusterDeploymentLogs(item.TaskID, 0)
	if err != nil || len(logs) != 4 || logs[3].Message != "ready" {
		t.Fatal(logs, err)
	}
	active, err = st.ActiveObservabilityMigrations()
	if err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
}
