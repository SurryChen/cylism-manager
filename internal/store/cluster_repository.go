package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

var ErrClusterDeploymentConflict = errors.New("cluster deployment conflict")
var ErrClusterHasNodes = errors.New("cluster has nodes or active tasks")

// StartClusterAgentOperation creates an agent maintenance task for the
// cluster control plane. Agent operations do not change node ownership.
func (s *Store) StartClusterAgentOperation(clusterID, serverID uint, operation string) (*model.ClusterDeployment, error) {
	task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "agent-" + operation, Role: "agent", Status: "running"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, clusterID).Error; err != nil {
			return err
		}
		if cluster.Distribution != "k3s" && cluster.Distribution != "k8s" {
			return ErrClusterDeploymentConflict
		}
		if cluster.ControlServerID == nil || *cluster.ControlServerID != serverID {
			return ErrClusterDeploymentConflict
		}
		var server model.Server
		if err := tx.Where("id = ? AND cluster_id = ? AND cluster_role = ?", serverID, clusterID, "control-plane").First(&server).Error; err != nil {
			return err
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", clusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return ErrClusterDeploymentConflict
		}
		switch operation {
		case "install", "upgrade", "restart", "stop", "uninstall", "test":
		default:
			return ErrClusterDeploymentConflict
		}
		return tx.Create(task).Error
	})
	return task, err
}

func (s *Store) FinishClusterAgentOperation(task *model.ClusterDeployment, success bool) error {
	status := "failed"
	if success {
		status = "success"
	}
	return s.db.Model(&model.ClusterDeployment{}).Where("id = ? AND status = ?", task.ID, "running").Update("status", status).Error
}

func (s *Store) ListClusters() ([]model.Cluster, error) {
	clusters := make([]model.Cluster, 0)
	err := s.db.Order("created_at desc, id desc").Find(&clusters).Error
	return clusters, err
}

func (s *Store) GetCluster(id uint) (*model.Cluster, error) {
	var cluster model.Cluster
	if err := s.db.First(&cluster, id).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (s *Store) GetClusterByName(name string) (*model.Cluster, error) {
	var cluster model.Cluster
	if err := s.db.Where("name = ?", name).First(&cluster).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (s *Store) CreateCluster(cluster *model.Cluster) error {
	return s.db.Create(cluster).Error
}

func (s *Store) UpdateClusterName(id uint, name string) (*model.Cluster, error) {
	var cluster model.Cluster
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&cluster, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&cluster).Update("name", name).Error; err != nil {
			return err
		}
		cluster.Name = name
		return nil
	})
	return &cluster, err
}

func (s *Store) DeleteCluster(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, id).Error; err != nil {
			return err
		}
		var nodes, running int64
		if err := tx.Model(&model.Server{}).Where("cluster_id = ?", id).Count(&nodes).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", id, "running").Count(&running).Error; err != nil {
			return err
		}
		if nodes > 0 || running > 0 || cluster.ControlServerID != nil {
			return ErrClusterHasNodes
		}
		if err := tx.Model(&model.AccessCertificate{}).Where("cluster_id = ? AND revoked_at IS NULL", id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "in_use": false}).Error; err != nil {
			return err
		}
		// Keep the audit trail after removing the logical cluster.
		return tx.Delete(&cluster).Error
	})
}

// StartClusterDeployment atomically reserves a server and one active deployment.
func (s *Store) StartClusterDeployment(clusterID, serverID uint, role, mirror, address string) (*model.ClusterDeployment, error) {
	deployment := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "deploy", Role: role, Mirror: mirror, Address: address, Status: "running"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, clusterID).Error; err != nil {
			return err
		}
		if cluster.Distribution != "k3s" {
			return ErrClusterDeploymentConflict
		}
		var server model.Server
		if err := tx.First(&server, serverID).Error; err != nil {
			return err
		}
		if server.ClusterID != nil || server.ClusterRole != "" || server.K8sNodeName != "" {
			return ErrClusterDeploymentConflict
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", clusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return ErrClusterDeploymentConflict
		}
		if role == "control-plane" {
			if cluster.ControlServerID != nil || cluster.Status == "ready" {
				return ErrClusterDeploymentConflict
			}
		} else if role == "worker" {
			if cluster.ControlServerID == nil || cluster.EncryptedToken == "" || cluster.Status != "ready" {
				return ErrClusterDeploymentConflict
			}
		} else {
			return ErrClusterDeploymentConflict
		}
		updated := tx.Model(&model.Server{}).Where("id = ? AND cluster_id IS NULL AND cluster_role = '' AND k8s_node_name = ''", serverID).Update("cluster_id", clusterID)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrClusterDeploymentConflict
		}
		if err := tx.Create(deployment).Error; err != nil {
			return err
		}
		if role == "control-plane" {
			return tx.Model(&model.Cluster{}).Where("id = ?", clusterID).Update("status", "deploying").Error
		}
		return nil
	})
	return deployment, err
}

// StartClusterAdoption reserves an existing K3s node without installing software.
func (s *Store) StartClusterAdoption(clusterID, serverID uint, role, address string) (*model.ClusterDeployment, error) {
	task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "adopt", Role: role, Address: address, Status: "running"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, clusterID).Error; err != nil {
			return err
		}
		if cluster.Distribution != "k3s" {
			return ErrClusterDeploymentConflict
		}
		var server model.Server
		if err := tx.First(&server, serverID).Error; err != nil {
			return err
		}
		if server.ClusterID != nil || server.ClusterRole != "" || server.K8sNodeName != "" {
			return ErrClusterDeploymentConflict
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", clusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return ErrClusterDeploymentConflict
		}
		switch role {
		case "control-plane":
			if cluster.ControlServerID != nil || cluster.Status == "ready" {
				return ErrClusterDeploymentConflict
			}
		case "worker":
			if cluster.ControlServerID == nil || cluster.Status != "ready" || cluster.EncryptedToken == "" {
				return ErrClusterDeploymentConflict
			}
		default:
			return ErrClusterDeploymentConflict
		}
		updated := tx.Model(&model.Server{}).Where("id = ? AND cluster_id IS NULL AND cluster_role = '' AND k8s_node_name = ''", serverID).Update("cluster_id", clusterID)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrClusterDeploymentConflict
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		if role == "control-plane" {
			return tx.Model(&model.Cluster{}).Where("id = ?", clusterID).Update("status", "deploying").Error
		}
		return nil
	})
	return task, err
}

// StartClusterUninstall reserves the cluster while a node is removed remotely.
func (s *Store) StartClusterUninstall(clusterID, serverID uint) (*model.ClusterDeployment, error) {
	task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "uninstall", Status: "running"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, clusterID).Error; err != nil {
			return err
		}
		if cluster.Distribution != "k3s" {
			return ErrClusterDeploymentConflict
		}
		var server model.Server
		if err := tx.Where("id = ? AND cluster_id = ?", serverID, clusterID).First(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrClusterDeploymentConflict
			}
			return err
		}
		if server.ClusterRole != "worker" && server.ClusterRole != "control-plane" {
			return ErrClusterDeploymentConflict
		}
		var running, workers int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", clusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrClusterDeploymentConflict
		}
		if server.ClusterRole == "control-plane" {
			if err := tx.Model(&model.Server{}).Where("cluster_id = ? AND id <> ?", clusterID, serverID).Count(&workers).Error; err != nil {
				return err
			}
			if workers > 0 {
				return ErrClusterDeploymentConflict
			}
		}
		task.Role = server.ClusterRole
		return tx.Create(task).Error
	})
	return task, err
}

func (s *Store) FinishClusterUninstall(task *model.ClusterDeployment, success bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		status := "failed"
		if success {
			status = "success"
		}
		if err := tx.Model(&model.ClusterDeployment{}).Where("id = ? AND status = ?", task.ID, "running").Update("status", status).Error; err != nil {
			return err
		}
		if !success {
			return nil
		}
		if err := tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ?", task.ServerID, task.ClusterID).Updates(map[string]any{"cluster_id": nil, "cluster_role": "", "k8s_node_name": ""}).Error; err != nil {
			return err
		}
		if task.Role == "control-plane" {
			return tx.Model(&model.Cluster{}).Where("id = ?", task.ClusterID).Updates(map[string]any{"status": "draft", "control_server_id": nil, "control_address": "", "encrypted_token": ""}).Error
		}
		return nil
	})
}

func (s *Store) ListClusterDeployments(clusterID uint) ([]model.ClusterDeployment, error) {
	var items []model.ClusterDeployment
	err := s.db.Where("cluster_id = ?", clusterID).Order("id desc").Find(&items).Error
	return items, err
}

func (s *Store) GetClusterDeployment(clusterID, deploymentID uint) (*model.ClusterDeployment, error) {
	var item model.ClusterDeployment
	if err := s.db.Where("cluster_id = ? AND id = ?", clusterID, deploymentID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) ListClusterDeploymentLogs(deploymentID, after uint) ([]model.ClusterDeploymentLog, error) {
	var items []model.ClusterDeploymentLog
	err := s.db.Where("deployment_id = ? AND id > ?", deploymentID, after).Order("id asc").Limit(300).Find(&items).Error
	return items, err
}

func (s *Store) AppendClusterDeploymentLog(deploymentID uint, message string) error {
	return s.db.Create(&model.ClusterDeploymentLog{DeploymentID: deploymentID, Message: message}).Error
}

func (s *Store) FinishClusterDeployment(deployment *model.ClusterDeployment, success bool, token, address, nodeName string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		status := "success"
		if !success {
			status = "failed"
		}
		if err := tx.Model(&model.ClusterDeployment{}).Where("id = ?", deployment.ID).Update("status", status).Error; err != nil {
			return err
		}
		if status == "failed" {
			if err := tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ?", deployment.ServerID, deployment.ClusterID).Update("cluster_id", nil).Error; err != nil {
				return err
			}
			if deployment.Role == "control-plane" {
				return tx.Model(&model.Cluster{}).Where("id = ?", deployment.ClusterID).Update("status", "failed").Error
			}
			return nil
		}
		if err := tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ?", deployment.ServerID, deployment.ClusterID).Updates(map[string]any{"cluster_role": deployment.Role, "k8s_node_name": nodeName}).Error; err != nil {
			return err
		}
		if deployment.Role == "control-plane" {
			return tx.Model(&model.Cluster{}).Where("id = ?", deployment.ClusterID).Updates(map[string]any{"status": "ready", "control_server_id": deployment.ServerID, "control_address": address, "encrypted_token": token}).Error
		}
		return nil
	})
}

// RecoverInterruptedClusterDeployments frees reservations left by a stopped API process.
func (s *Store) RecoverInterruptedClusterDeployments() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var running []model.ClusterDeployment
		if err := tx.Where("status = ?", "running").Find(&running).Error; err != nil {
			return err
		}
		for _, task := range running {
			if strings.HasPrefix(task.Operation, "agent-") {
				if err := tx.Model(&model.ClusterDeployment{}).Where("id = ?", task.ID).Update("status", "failed").Error; err != nil {
					return err
				}
				if err := tx.Create(&model.ClusterDeploymentLog{DeploymentID: task.ID, Message: "API 服务重启，Agent 运维任务已中断；请检查控制平面上的 Agent 服务"}).Error; err != nil {
					return err
				}
				if err := tx.Create(&model.AuditLog{Action: "cluster.agent", ResourceType: "cluster", ResourceID: task.ClusterID, ActorType: model.AuditActorSystem, Source: model.AuditSourceSystem, Outcome: model.AuditOutcomeFailed, Summary: "API 服务重启，Agent 运维任务已中断", OperationID: fmt.Sprintf("cluster-deployment-%d", task.ID), Detail: fmt.Sprintf(`{"deployment_id":%d,"server_id":%d,"operation":%q}`, task.ID, task.ServerID, task.Operation)}).Error; err != nil {
					return err
				}
				continue
			}
			if task.Operation == "uninstall" {
				if err := tx.Model(&model.ClusterDeployment{}).Where("id = ?", task.ID).Update("status", "failed").Error; err != nil {
					return err
				}
				if err := tx.Create(&model.ClusterDeploymentLog{DeploymentID: task.ID, Message: "API 服务重启，卸载已中断；请核对目标机器后重试"}).Error; err != nil {
					return err
				}
				if err := tx.Create(&model.AuditLog{
					Action: "cluster.uninstall", ResourceType: "cluster", ResourceID: task.ClusterID,
					ActorType: model.AuditActorSystem, Source: model.AuditSourceSystem, Outcome: model.AuditOutcomeFailed,
					Summary:     "API 服务重启，节点卸载任务已中断",
					OperationID: fmt.Sprintf("cluster-deployment-%d", task.ID),
					Detail:      fmt.Sprintf(`{"deployment_id":%d,"server_id":%d}`, task.ID, task.ServerID),
				}).Error; err != nil {
					return err
				}
				continue
			}
			action := "cluster.deploy"
			message := "API 服务重启，部署已中断；请核对目标机器的 K3s 状态后重试"
			if task.Operation == "adopt" {
				action = "cluster.adopt"
				message = "API 服务重启，纳管已中断；请核对目标机器后重试"
			}
			if err := tx.Model(&model.ClusterDeployment{}).Where("id = ?", task.ID).Update("status", "failed").Error; err != nil {
				return err
			}
			if err := tx.Create(&model.ClusterDeploymentLog{DeploymentID: task.ID, Message: message}).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AuditLog{
				Action: action, ResourceType: "cluster", ResourceID: task.ClusterID,
				ActorType: model.AuditActorSystem,
				Source:    model.AuditSourceSystem, Outcome: model.AuditOutcomeFailed,
				Summary:     "API 服务重启，集群部署任务已中断",
				OperationID: fmt.Sprintf("cluster-deployment-%d", task.ID),
				Detail:      fmt.Sprintf(`{"deployment_id":%d,"server_id":%d}`, task.ID, task.ServerID),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ? AND cluster_role = ''", task.ServerID, task.ClusterID).Update("cluster_id", nil).Error; err != nil {
				return err
			}
			if task.Role == "control-plane" {
				if err := tx.Model(&model.Cluster{}).Where("id = ? AND status = ?", task.ClusterID, "deploying").Update("status", "failed").Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
