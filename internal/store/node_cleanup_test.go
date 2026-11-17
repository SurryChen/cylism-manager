package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func removedWorkerFixture(t *testing.T) (*Store, *model.Cluster, *model.Server, *model.ClusterDeployment) {
	t.Helper()
	st, cluster, _, worker := removalFixture(t)
	task, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); err != nil {
		t.Fatal(err)
	}
	return st, cluster, worker, task
}

func TestRemovedWorkerCanBeCleanedWithoutClusterAndRetainsCredentials(t *testing.T) {
	st, cluster, worker, removal := removedWorkerFixture(t)
	server, _ := st.GetServer(worker.ID)
	if server.K3sRemovalTaskID == nil || *server.K3sRemovalTaskID != removal.ID {
		t.Fatal("missing standalone cleanup entry")
	}
	if _, err := st.StartServerK3sCleanup(worker.ID, removal.ID+1); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("stale confirmation accepted: %v", err)
	}
	// The original cluster can disappear; cleanup relies on saved host identity.
	if err := st.DB().Delete(cluster).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartServerK3sCleanup(worker.ID, removal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !task.UninstallK3s || !task.NodeStopped || task.NodeUID != removal.NodeUID || task.NodeMachineID != removal.NodeMachineID {
		t.Fatalf("lost identity: %#v", task)
	}
	if _, err := st.StartServerK3sCleanup(worker.ID, removal.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("concurrent cleanup: %v", err)
	}
	if err := st.UpdateServer(server); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("edited reserved server: %v", err)
	}
	if err := st.DeleteServer(worker.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("deleted reserved server: %v", err)
	}
	if err := st.FinishServerK3sCleanup(task, true); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("completion without verification: %v", err)
	}
	if err := st.MarkClusterNodeUninstalled(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishServerK3sCleanup(task, true); err != nil {
		t.Fatal(err)
	}
	server, _ = st.GetServer(worker.ID)
	if server.K3sRemovalTaskID != nil || server.ClusterID != nil || server.SSHKey != worker.SSHKey || server.SSHPassword != worker.SSHPassword {
		t.Fatalf("unexpected server change: %#v", server)
	}
	if _, err := st.StartServerK3sCleanup(worker.ID, removal.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("completed cleanup repeated: %v", err)
	}
}

func TestCleanupFailureAndRestartKeepStandaloneEntryAndBlockReuse(t *testing.T) {
	st, cluster, worker, removal := removedWorkerFixture(t)
	if _, err := st.StartServerK3sCleanup(1, removal.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("unrelated server: %v", err)
	}
	if _, err := st.StartClusterAdoption(cluster.ID, worker.ID, "worker", ""); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("adoption before cleanup: %v", err)
	}
	if err := st.DB().Model(cluster).Update("encrypted_token", "token").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterDeployment(cluster.ID, worker.ID, "worker", "official", "", "public"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("deployment before cleanup: %v", err)
	}
	task, err := st.StartServerK3sCleanup(worker.ID, removal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeUninstalled(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	latest, _ := st.LatestServerK3sCleanup(worker.ID)
	server, _ := st.GetServer(worker.ID)
	if latest.Status != "failed" || !latest.NodeUninstalled || server.K3sRemovalTaskID == nil {
		t.Fatalf("lost restart checkpoint: %#v %#v", latest, server)
	}
	var audit model.AuditLog
	if err := st.DB().Where("action = ? AND resource_type = ? AND resource_id = ?", "cluster.node_cleanup", "server", worker.ID).First(&audit).Error; err != nil {
		t.Fatalf("missing server cleanup recovery audit: %v", err)
	}
	retry, err := st.StartServerK3sCleanup(worker.ID, removal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeUninstalled(retry.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishServerK3sCleanup(retry, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterAdoption(cluster.ID, worker.ID, "worker", ""); err != nil {
		t.Fatalf("reuse after cleanup: %v", err)
	}
}

func TestRemovalWithUninstallRequiresVerifiedCleanupBeforeUnbinding(t *testing.T) {
	st, cluster, _, worker := removalFixture(t)
	identity := removalIdentity()
	identity.UninstallK3s = true
	task, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("unverified uninstall accepted: %v", err)
	}
	server, _ := st.GetServer(worker.ID)
	if server.ClusterID == nil {
		t.Fatal("unbound before cleanup")
	}
	if err = st.MarkClusterNodeUninstalled(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); err != nil {
		t.Fatal(err)
	}
	server, _ = st.GetServer(worker.ID)
	if server.ClusterID != nil || server.K3sRemovalTaskID != nil {
		t.Fatal("clean removal left a binding or redundant cleanup entry")
	}
}

func TestCleanupMigrationOnlyBackfillsReliableFinalRemoval(t *testing.T) {
	st, _, worker, removal := removedWorkerFixture(t)
	other := &model.Server{Name: "reused", Host: "192.0.2.9"}
	if err := st.DB().Create(other).Error; err != nil {
		t.Fatal(err)
	}
	oldRemoval := *removal
	oldRemoval.ID, oldRemoval.ServerID, oldRemoval.Status, oldRemoval.NodeStopped = 0, other.ID, "success", true
	if err := st.DB().Create(&oldRemoval).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Create(&model.ClusterDeployment{ServerID: other.ID, ClusterID: removal.ClusterID, Operation: "uninstall", Status: "success"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Migrator().DropColumn(&model.Server{}, "K3sRemovalTaskID"); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"UninstallK3s", "NodeUninstalled"} {
		if err := st.DB().Migrator().DropColumn(&model.ClusterDeployment{}, column); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateSchema(st.DB()); err != nil {
		t.Fatal(err)
	}
	server, _ := st.GetServer(worker.ID)
	reused, _ := st.GetServer(other.ID)
	if server.K3sRemovalTaskID == nil || *server.K3sRemovalTaskID != removal.ID || reused.K3sRemovalTaskID != nil {
		t.Fatalf("unsafe backfill: %#v %#v", server, reused)
	}
	if err := st.DB().Model(server).Update("k3s_removal_task_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema(st.DB()); err != nil {
		t.Fatal(err)
	}
	server, _ = st.GetServer(worker.ID)
	if server.K3sRemovalTaskID != nil {
		t.Fatal("repeat migration restored cleared marker")
	}
}
