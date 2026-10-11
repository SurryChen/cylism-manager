package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestClusterNetworkMigrationKeepsNodeOwnershipAndCommitsOnSuccess(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.Cluster{Name: "migration", Distribution: "k3s", Status: "ready", NetworkMode: "public", ControlAddress: "203.0.113.10"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "control", Host: "203.0.113.10", ClusterID: &cluster.ID, ClusterRole: "control-plane", K8sNodeName: "control"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(&cluster).Update("control_server_id", server.ID).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterNetworkMigration(cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterNetworkMigration(cluster.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("concurrent start: %v", err)
	}
	if err := st.FinishClusterNetworkMigration(task, false, ""); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetCluster(cluster.ID)
	if err != nil || current.NetworkMode != "public" || current.ControlAddress != "203.0.113.10" {
		t.Fatalf("failed migration changed cluster: %#v %v", current, err)
	}
	owned, err := st.GetServer(server.ID)
	if err != nil || owned.ClusterID == nil || *owned.ClusterID != cluster.ID {
		t.Fatalf("failed migration unbound node: %#v %v", owned, err)
	}
	task, err = st.StartClusterNetworkMigration(cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterNetworkMigration(task, true, "100.81.23.123"); err != nil {
		t.Fatal(err)
	}
	current, err = st.GetCluster(cluster.ID)
	if err != nil || current.NetworkMode != "tailscale" || current.ControlAddress != "203.0.113.10" || current.TailscaleIP != "100.81.23.123" {
		t.Fatalf("successful migration not committed: %#v %v", current, err)
	}
}

func TestInterruptedNetworkMigrationDoesNotReleaseNodes(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cluster := model.Cluster{Name: "interrupted", Distribution: "k3s", Status: "ready", NetworkMode: "public"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "control", Host: "203.0.113.10", ClusterID: &cluster.ID, ClusterRole: "control-plane", K8sNodeName: "control"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(&cluster).Update("control_server_id", server.ID).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterNetworkMigration(cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	owned, err := st.GetServer(server.ID)
	if err != nil || owned.ClusterID == nil || *owned.ClusterID != cluster.ID {
		t.Fatalf("interrupted migration unbound node: %#v %v", owned, err)
	}
	stored, err := st.GetClusterDeployment(cluster.ID, task.ID)
	if err != nil || stored.Status != "failed" {
		t.Fatalf("task=%#v err=%v", stored, err)
	}
}
