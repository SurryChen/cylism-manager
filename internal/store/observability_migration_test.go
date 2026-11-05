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
	if err != nil || len(logs) != 2 || logs[1].Message != "ready" {
		t.Fatal(logs, err)
	}
	active, err = st.ActiveObservabilityMigrations()
	if err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
}
