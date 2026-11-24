package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func removalFixture(t *testing.T) (*Store, *model.Cluster, *model.Server, *model.Server) {
	t.Helper()
	st := setupTestDB(t)
	cluster := &model.Cluster{Name: "remove", Distribution: "k3s", Status: "ready"}
	if err := st.DB().Create(cluster).Error; err != nil {
		t.Fatal(err)
	}
	control := &model.Server{Name: "control", Host: "192.0.2.1", ClusterID: &cluster.ID, ClusterRole: "control-plane", K8sNodeName: "control"}
	worker := &model.Server{Name: "worker", Host: "192.0.2.2", ClusterID: &cluster.ID, ClusterRole: "worker", K8sNodeName: "worker", SSHKey: "encrypted-key", SSHPassword: "encrypted-password"}
	for _, v := range []any{control, worker} {
		if err := st.DB().Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DB().Model(cluster).Update("control_server_id", control.ID).Error; err != nil {
		t.Fatal(err)
	}
	return st, cluster, control, worker
}

func removalIdentity() *model.ClusterDeployment {
	return &model.ClusterDeployment{NodeName: "worker", NodeUID: "uid-1", NodeMachineID: "machine-1", NodePodCIDRs: `["10.42.1.0/24"]`}
}

func TestClusterNodeRemovalIsScopedAndPreservesServer(t *testing.T) {
	st, cluster, control, worker := removalFixture(t)
	other := &model.Cluster{Name: "other", Distribution: "k3s", Status: "ready", ControlServerID: &control.ID}
	if err := st.DB().Create(other).Error; err != nil {
		t.Fatal(err)
	}
	sameName := &model.Server{Name: "other-worker", ClusterID: &other.ID, ClusterRole: "worker", K8sNodeName: "worker"}
	if err := st.DB().Create(sameName).Error; err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][2]uint{{other.ID, worker.ID}, {cluster.ID, control.ID}} {
		if _, err := st.StartClusterNodeRemoval(ids[0], ids[1], removalIdentity()); !errors.Is(err, ErrClusterDeploymentConflict) {
			t.Fatalf("unsafe reservation: %v", err)
		}
	}
	if _, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, nil); err == nil {
		t.Fatal("nil identity accepted")
	}
	task, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity()); err == nil {
		t.Fatal("concurrent task accepted")
	}
	if err := st.FinishClusterNodeRemoval(task, false); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetServer(worker.ID)
	if got.ClusterID == nil || got.K8sNodeName != "worker" {
		t.Fatal("failure cleared binding")
	}
	task, err = st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetServer(worker.ID)
	if got.ClusterID != nil || got.ClusterRole != "" || got.K8sNodeName != "" || got.SSHKey != worker.SSHKey || got.SSHPassword != worker.SSHPassword || got.Host != worker.Host {
		t.Fatalf("server changed: %#v", got)
	}
	for _, unchanged := range []*model.Server{control, sameName} {
		got, _ = st.GetServer(unchanged.ID)
		if got.ClusterID == nil || *got.ClusterID != *unchanged.ClusterID || got.ClusterRole != unchanged.ClusterRole || got.K8sNodeName != unchanged.K8sNodeName {
			t.Fatalf("unrelated binding changed: %#v", got)
		}
	}
	latest, err := st.LatestClusterNodeRemoval(cluster.ID, worker.ID)
	if err != nil || latest.Status != "success" || !latest.NodeStopped || latest.NodeUID != "uid-1" {
		t.Fatalf("task: %#v, %v", latest, err)
	}
}

func TestClusterNodeRemovalCompletionRollsBackIfBindingChanged(t *testing.T) {
	st, cluster, _, worker := removalFixture(t)
	task, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); err == nil {
		t.Fatal("completion without stopped checkpoint accepted")
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.DB().Model(worker).Update("k8s_node_name", "replacement").Error; err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, true); err == nil {
		t.Fatal("changed binding accepted")
	}
	persisted, _ := st.GetClusterDeployment(cluster.ID, task.ID)
	if persisted.Status != "running" {
		t.Fatalf("transaction did not roll back: %s", persisted.Status)
	}
	if err = st.FinishClusterNodeRemoval(task, false); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedClusterNodeRemovalRetainsBindingAndCheckpoint(t *testing.T) {
	st, cluster, _, worker := removalFixture(t)
	task, err := st.StartClusterNodeRemoval(cluster.ID, worker.ID, removalIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	latest, _ := st.LatestClusterNodeRemoval(cluster.ID, worker.ID)
	got, _ := st.GetServer(worker.ID)
	logs, _ := st.ListClusterDeploymentLogs(task.ID, 0)
	if latest.Status != "failed" || !latest.NodeStopped || got.ClusterID == nil || len(logs) == 0 {
		t.Fatalf("recovery lost state: %#v %#v %#v", latest, got, logs)
	}
	var audit model.AuditLog
	if err = st.DB().Where("action = ?", "cluster.node_remove").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRemovalMigrationPreservesLegacyDeployment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE cluster_deployments (id integer PRIMARY KEY, cluster_id integer NOT NULL, server_id integer NOT NULL, operation text NOT NULL DEFAULT 'deploy', role text NOT NULL, mirror text NOT NULL, network_mode text NOT NULL DEFAULT 'public', address text, status text NOT NULL, created_at datetime, updated_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO cluster_deployments (id,cluster_id,server_id,operation,role,mirror,status) VALUES (1,1,1,'deploy','worker','official','success')`).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := db.DB()
	_ = raw.Close()
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.GetClusterDeployment(1, 1)
	if err != nil || task.Status != "success" || task.NodeStopped || task.NodeUID != "" {
		t.Fatalf("migration changed history: %#v %v", task, err)
	}
	for _, column := range []string{"NodeUID", "NodeMachineID", "NodePodCIDRs", "NodeStopped"} {
		if !st.DB().Migrator().HasColumn(&model.ClusterDeployment{}, column) {
			t.Fatalf("missing migration: %s", column)
		}
	}
}
