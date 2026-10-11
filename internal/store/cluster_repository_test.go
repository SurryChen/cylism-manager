package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestListClustersReturnsPersistedRecords(t *testing.T) {
	st := setupTestDB(t)
	clusters, err := st.ListClusters()
	if err != nil || clusters == nil || len(clusters) != 0 {
		t.Fatalf("empty clusters = %#v, err=%v", clusters, err)
	}
	cluster := model.Cluster{Name: "生产集群", Distribution: "k3s", Status: "draft"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	clusters, err = st.ListClusters()
	if err != nil || len(clusters) != 1 || clusters[0].ID != cluster.ID || clusters[0].Name != cluster.Name {
		t.Fatalf("persisted clusters = %#v, err=%v", clusters, err)
	}
	if _, err := st.GetCluster(cluster.ID); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if _, err := st.GetClusterByName(cluster.Name); err != nil {
		t.Fatalf("get cluster by name: %v", err)
	}
	if err := st.CreateCluster(&model.Cluster{Name: cluster.Name, Distribution: "k3s"}); err == nil {
		t.Fatal("cluster names must be unique")
	}
}

func TestClusterDeploymentReservesServerAndPersistsLogs(t *testing.T) {
	st := setupTestDB(t)
	first := model.Cluster{Name: "集群一", Distribution: "k3s", Status: "draft"}
	second := model.Cluster{Name: "集群二", Distribution: "k3s", Status: "draft"}
	server := model.Server{Name: "机器一", Host: "10.0.0.1"}
	worker := model.Server{Name: "机器二", Host: "10.0.0.2"}
	for _, item := range []any{&first, &second, &server, &worker} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.StartClusterDeployment(first.ID, worker.ID, "worker", "cn", "", "public"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("worker before control = %v", err)
	}
	task, err := st.StartClusterDeployment(first.ID, server.ID, "control-plane", "cn", "100.81.23.123", "public")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterDeployment(second.ID, server.ID, "control-plane", "cn", "10.0.0.1", "public"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("same server in another cluster = %v", err)
	}
	if _, err := st.StartClusterDeployment(first.ID, worker.ID, "worker", "cn", "", "public"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("concurrent deployment = %v", err)
	}
	if err := st.AppendClusterDeploymentLog(task.ID, "安装中"); err != nil {
		t.Fatal(err)
	}
	logs, err := st.ListClusterDeploymentLogs(task.ID, 0)
	if err != nil || len(logs) != 1 || logs[0].Message != "安装中" {
		t.Fatalf("logs = %#v, %v", logs, err)
	}
	if err := st.FinishClusterDeployment(task, true, "encrypted-token", "100.81.23.123", "control-a"); err != nil {
		t.Fatal(err)
	}
	cluster, err := st.GetCluster(first.ID)
	if err != nil || cluster.Status != "ready" || cluster.ControlAddress != server.Host || cluster.TailscaleIP != "" || cluster.EncryptedToken != "encrypted-token" {
		t.Fatalf("cluster = %#v, %v", cluster, err)
	}
	workerTask, err := st.StartClusterDeployment(first.ID, worker.ID, "worker", "official", "", "public")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterDeployment(workerTask, false, "", "", ""); err != nil {
		t.Fatal(err)
	}
	workerAfter, err := st.GetServer(worker.ID)
	if err != nil || workerAfter.ClusterID != nil {
		t.Fatalf("failed worker reservation = %#v, %v", workerAfter, err)
	}
}

func TestClusterDeploymentKeepsTailscaleModeForWorkers(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "private", Distribution: "k3s", Status: "draft"}
	control := model.Server{Name: "control", Host: "203.0.113.1"}
	worker := model.Server{Name: "worker", Host: "203.0.113.2"}
	for _, item := range []any{&cluster, &control, &worker} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	task, err := st.StartClusterDeployment(cluster.ID, control.ID, "control-plane", "official", "100.81.23.123", "tailscale")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterDeployment(task, true, "encrypted-token", "100.81.23.123", "control"); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetCluster(cluster.ID)
	if err != nil || stored.NetworkMode != "tailscale" || stored.ControlAddress != control.Host || stored.TailscaleIP != "100.81.23.123" {
		t.Fatalf("cluster network mode = %#v, %v", stored, err)
	}
	if _, err := st.StartClusterDeployment(cluster.ID, worker.ID, "worker", "official", "", "public"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("expected mode conflict, got %v", err)
	}
	if _, err := st.StartClusterDeployment(cluster.ID, worker.ID, "worker", "official", "", "tailscale"); err != nil {
		t.Fatalf("tailscale worker reservation: %v", err)
	}
}

func TestClusterNodeRestartTracksTaskWithoutChangingNodeOwnership(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "restart", Distribution: "k3s", Status: "ready"}
	other := model.Cluster{Name: "other", Distribution: "k3s", Status: "ready"}
	for _, item := range []*model.Cluster{&cluster, &other} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	control := model.Server{Name: "control", Host: "203.0.113.1", ClusterID: &cluster.ID, ClusterRole: "control-plane", K8sNodeName: "control"}
	worker := model.Server{Name: "worker", Host: "203.0.113.2", ClusterID: &cluster.ID, ClusterRole: "worker", K8sNodeName: "worker"}
	for _, item := range []*model.Server{&control, &worker} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DB().Model(&cluster).Update("control_server_id", control.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterNodeRestart(other.ID, worker.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("cross-cluster restart: %v", err)
	}
	task, err := st.StartClusterNodeRestart(cluster.ID, worker.ID)
	if err != nil || task.Role != "worker" || task.Operation != "restart" {
		t.Fatalf("worker restart task: %#v, %v", task, err)
	}
	if _, err := st.StartClusterNodeRestart(cluster.ID, control.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("parallel restart: %v", err)
	}
	if err := st.FinishClusterNodeRestart(task, false); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetClusterDeployment(cluster.ID, task.ID)
	if err != nil || stored.Status != "failed" {
		t.Fatalf("failed task: %#v, %v", stored, err)
	}
	controlTask, err := st.StartClusterNodeRestart(cluster.ID, control.ID)
	if err != nil || controlTask.Role != "control-plane" {
		t.Fatalf("control restart task: %#v, %v", controlTask, err)
	}
	if err := st.FinishClusterNodeRestart(controlTask, true); err != nil {
		t.Fatal(err)
	}
	for _, item := range []*model.Server{&control, &worker} {
		got, err := st.GetServer(item.ID)
		if err != nil || got.ClusterID == nil || *got.ClusterID != cluster.ID || got.ClusterRole != item.ClusterRole {
			t.Fatalf("restart changed node ownership: %#v, %v", got, err)
		}
	}
	interrupted, err := st.StartClusterNodeRestart(cluster.ID, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	stored, err = st.GetClusterDeployment(cluster.ID, interrupted.ID)
	if err != nil || stored.Status != "failed" {
		t.Fatalf("interrupted restart: %#v, %v", stored, err)
	}
	got, err := st.GetServer(worker.ID)
	if err != nil || got.ClusterID == nil || *got.ClusterID != cluster.ID {
		t.Fatalf("interrupted restart changed ownership: %#v, %v", got, err)
	}
}

func TestRecoverInterruptedClusterDeployment(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "集群", Distribution: "k3s", Status: "draft"}
	server := model.Server{Name: "机器", Host: "10.0.0.1"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterDeployment(cluster.ID, server.ID, "control-plane", "official", server.Host, "public")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverInterruptedClusterDeployments(); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetClusterDeployment(cluster.ID, task.ID)
	if err != nil || updated.Status != "failed" {
		t.Fatalf("recovered task = %#v, %v", updated, err)
	}
	serverAfter, err := st.GetServer(server.ID)
	if err != nil || serverAfter.ClusterID != nil {
		t.Fatalf("recovered server = %#v, %v", serverAfter, err)
	}
	events, total, err := st.ListAuditLogsFiltered(model.AuditLogFilter{ResourceType: "cluster", Action: "cluster.deploy", Limit: 10})
	if err != nil || total != 1 || len(events) != 1 || events[0].Outcome != model.AuditOutcomeFailed {
		t.Fatalf("interrupted deployment audit = %#v, total=%d, err=%v", events, total, err)
	}
}

func TestClusterDeleteRequiresAllNodesUninstalled(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "待删除", Distribution: "k3s", Status: "ready", NetworkMode: "tailscale", ControlAddress: "10.0.0.1", TailscaleIP: "100.81.23.123"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "控制节点", Host: "10.0.0.1", ClusterID: &cluster.ID, ClusterRole: "control-plane"}
	worker := model.Server{Name: "工作节点", Host: "10.0.0.2", ClusterID: &cluster.ID, ClusterRole: "worker"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Create(&worker).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(&cluster).Update("control_server_id", server.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCluster(cluster.ID); !errors.Is(err, ErrClusterHasNodes) {
		t.Fatalf("delete with nodes = %v", err)
	}
	if _, err := st.StartClusterUninstall(cluster.ID, server.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("control before worker = %v", err)
	}
	workerTask, err := st.StartClusterUninstall(cluster.ID, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartClusterUninstall(cluster.ID, server.ID); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("parallel uninstall = %v", err)
	}
	if err := st.FinishClusterUninstall(workerTask, true); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCluster(cluster.ID); !errors.Is(err, ErrClusterHasNodes) {
		t.Fatalf("delete with control = %v", err)
	}
	controlTask, err := st.StartClusterUninstall(cluster.ID, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterUninstall(controlTask, true); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetCluster(cluster.ID)
	if err != nil || updated.Status != "draft" || updated.ControlServerID != nil || updated.ControlAddress != "" || updated.TailscaleIP != "" || updated.EncryptedToken != "" {
		t.Fatalf("cluster after uninstall = %#v, %v", updated, err)
	}
	if err := st.DeleteCluster(cluster.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCluster(cluster.ID); err == nil {
		t.Fatal("cluster still exists")
	}
}

func TestClusterDeleteRevokesAccessCertificates(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "待撤销证书的集群", Distribution: "k3s", Status: "draft"}
	if err := st.CreateCluster(&cluster); err != nil {
		t.Fatal(err)
	}
	cert := model.AccessCertificate{ClusterID: cluster.ID, Kind: "platform_client", Serial: "1234", Subject: "client", CertificatePEM: "cert", Fingerprint: "fingerprint"}
	if err := st.CreateAccessCertificate(&cert); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCluster(cluster.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetAccessCertificate(cert.ID)
	if err != nil || stored.RevokedAt == nil || stored.InUse {
		t.Fatalf("certificate after cluster deletion = %#v, err=%v", stored, err)
	}
}

func TestFailedUninstallKeepsNodeBound(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "故障集群", Distribution: "k3s", Status: "ready"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "节点", Host: "10.0.0.1", ClusterID: &cluster.ID, ClusterRole: "worker"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterUninstall(cluster.ID, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterUninstall(task, false); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetServer(server.ID)
	if err != nil || updated.ClusterID == nil || updated.ClusterRole != "worker" {
		t.Fatalf("failed uninstall unbound node = %#v, %v", updated, err)
	}
	if err := st.DeleteCluster(cluster.ID); !errors.Is(err, ErrClusterHasNodes) {
		t.Fatalf("delete after failed uninstall = %v", err)
	}
}

func TestClusterDeleteWaitsForRunningTask(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "运行任务", Distribution: "k3s", Status: "draft"}
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	task := model.ClusterDeployment{ClusterID: cluster.ID, ServerID: 1, Operation: "deploy", Role: "control-plane", Status: "running"}
	if err := st.DB().Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCluster(cluster.ID); !errors.Is(err, ErrClusterHasNodes) {
		t.Fatalf("delete while running = %v", err)
	}
}

func TestClusterAdoptionReservesExistingNodes(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "已有集群", Distribution: "k3s", Status: "draft"}
	control := model.Server{Name: "现有控制平面", Host: "10.0.0.1"}
	worker := model.Server{Name: "现有工作节点", Host: "10.0.0.2"}
	for _, item := range []any{&cluster, &control, &worker} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.StartClusterAdoption(cluster.ID, worker.ID, "worker", ""); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("worker before control = %v", err)
	}
	task, err := st.StartClusterAdoption(cluster.ID, control.ID, "control-plane", control.Host)
	if err != nil {
		t.Fatal(err)
	}
	if task.Operation != "adopt" {
		t.Fatalf("operation = %q", task.Operation)
	}
	if _, err := st.StartClusterAdoption(cluster.ID, worker.ID, "worker", ""); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("parallel adoption = %v", err)
	}
	if err := st.FinishClusterDeployment(task, true, "encrypted-token", control.Host, "control-node"); err != nil {
		t.Fatal(err)
	}
	workerTask, err := st.StartClusterAdoption(cluster.ID, worker.ID, "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterDeployment(workerTask, false, "", "", ""); err != nil {
		t.Fatal(err)
	}
	workerAfter, err := st.GetServer(worker.ID)
	if err != nil || workerAfter.ClusterID != nil {
		t.Fatalf("failed adoption reserved worker = %#v, %v", workerAfter, err)
	}
	workerTask, err = st.StartClusterAdoption(cluster.ID, worker.ID, "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterDeployment(workerTask, true, "", "", "worker-node"); err != nil {
		t.Fatal(err)
	}
	workerAfter, err = st.GetServer(worker.ID)
	if err != nil || workerAfter.ClusterID == nil || *workerAfter.ClusterID != cluster.ID || workerAfter.K8sNodeName != "worker-node" {
		t.Fatalf("adopted worker = %#v, %v", workerAfter, err)
	}
}

func TestClusterAgentOperationReservesControlPlane(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	server := model.Server{Name: "control", Host: "10.0.0.1", ClusterRole: "control-plane"}
	cluster := model.Cluster{Name: "agent-cluster", Distribution: "k3s", Status: "ready"}
	if err := st.DB().Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	cluster.ControlServerID = &server.ID
	if err := st.DB().Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Model(&server).Update("cluster_id", cluster.ID).Error; err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterAgentOperation(cluster.ID, server.ID, "install")
	if err != nil {
		t.Fatal(err)
	}
	if task.Operation != "agent-install" || task.Role != "agent" {
		t.Fatalf("task = %#v", task)
	}
	if _, err := st.StartClusterAgentOperation(cluster.ID, server.ID, "test"); !errors.Is(err, ErrClusterDeploymentConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := st.AppendClusterDeploymentLog(task.ID, "agent ok"); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishClusterAgentOperation(task, true); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetClusterDeployment(cluster.ID, task.ID)
	if err != nil || stored.Status != "success" {
		t.Fatalf("stored = %#v, err=%v", stored, err)
	}
}
