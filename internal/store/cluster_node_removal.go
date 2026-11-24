package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// StartClusterNodeRemoval reserves only a bound worker; identity is frozen for retries.
func (s *Store) StartClusterNodeRemoval(clusterID, serverID uint, identity *model.ClusterDeployment) (*model.ClusterDeployment, error) {
	if identity == nil {
		return nil, ErrClusterDeploymentConflict
	}
	task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "remove", Role: "worker", Status: "running", NodeName: identity.NodeName, NodeUID: identity.NodeUID, NodeMachineID: identity.NodeMachineID, NodePodCIDRs: identity.NodePodCIDRs, NodeStopped: identity.NodeStopped, UninstallK3s: identity.UninstallK3s, NodeUninstalled: identity.NodeUninstalled}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.Cluster
		if err := tx.First(&cluster, clusterID).Error; err != nil {
			return err
		}
		if cluster.Distribution != "k3s" || cluster.Status != "ready" || cluster.ControlServerID == nil || *cluster.ControlServerID == serverID {
			return ErrClusterDeploymentConflict
		}
		var server model.Server
		if err := tx.Where("id = ? AND cluster_id = ?", serverID, clusterID).First(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrClusterDeploymentConflict
			}
			return err
		}
		if server.ClusterRole != "worker" || server.K8sNodeName == "" || server.K8sNodeName != task.NodeName || task.NodeUID == "" || task.NodeMachineID == "" {
			return ErrClusterDeploymentConflict
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", clusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return ErrClusterDeploymentConflict
		}
		return tx.Create(task).Error
	})
	return task, err
}

func (s *Store) LatestClusterNodeRemoval(clusterID, serverID uint) (*model.ClusterDeployment, error) {
	var task model.ClusterDeployment
	err := s.db.Where("cluster_id = ? AND server_id = ? AND operation = ?", clusterID, serverID, "remove").Order("id desc").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}

// FinishClusterNodeRemoval atomically completes the task and clears exactly its worker binding.
func (s *Store) FinishClusterNodeRemoval(task *model.ClusterDeployment, success bool) error {
	if task == nil {
		return ErrClusterDeploymentConflict
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var persisted model.ClusterDeployment
		if err := tx.Where("id = ? AND cluster_id = ? AND server_id = ? AND operation = ? AND status = ?", task.ID, task.ClusterID, task.ServerID, "remove", "running").First(&persisted).Error; err != nil {
			return err
		}
		if success && (!persisted.NodeStopped || (persisted.UninstallK3s && !persisted.NodeUninstalled)) {
			return ErrClusterDeploymentConflict
		}
		status := "failed"
		if success {
			status = "success"
		}
		result := tx.Model(&model.ClusterDeployment{}).Where("id = ? AND cluster_id = ? AND server_id = ? AND operation = ? AND status = ?", task.ID, task.ClusterID, task.ServerID, "remove", "running").Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrClusterDeploymentConflict
		}
		if !success {
			return nil
		}
		var cleanupTaskID *uint
		if !persisted.NodeUninstalled {
			cleanupTaskID = &persisted.ID
		}
		result = tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ? AND cluster_role = ? AND k8s_node_name = ?", persisted.ServerID, persisted.ClusterID, "worker", persisted.NodeName).Updates(map[string]any{"cluster_id": nil, "cluster_role": "", "k8s_node_name": "", "k3s_removal_task_id": cleanupTaskID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrClusterDeploymentConflict
		}
		return nil
	})
}

func (s *Store) MarkClusterNodeRemovalStopped(taskID uint) error {
	result := s.db.Model(&model.ClusterDeployment{}).Where("id = ? AND operation = ? AND status = ?", taskID, "remove", "running").Update("node_stopped", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClusterDeploymentConflict
	}
	return nil
}

func (s *Store) MarkClusterNodeUninstalled(taskID uint) error {
	result := s.db.Model(&model.ClusterDeployment{}).Where("id = ? AND operation IN ? AND status = ? AND node_stopped = ? AND uninstall_k3s = ?", taskID, []string{"remove", "cleanup"}, "running", true, true).Update("node_uninstalled", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClusterDeploymentConflict
	}
	return nil
}

// A cleanup reservation retains the original machine identity even after the
// logical cluster has been deleted. It cannot target an arbitrary unbound host.
func (s *Store) StartServerK3sCleanup(serverID, removalTaskID uint) (*model.ClusterDeployment, error) {
	var task model.ClusterDeployment
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var server model.Server
		if err := tx.First(&server, serverID).Error; err != nil {
			return err
		}
		if server.ClusterID != nil || server.ClusterRole != "" || server.K8sNodeName != "" || server.K3sRemovalTaskID == nil || *server.K3sRemovalTaskID != removalTaskID {
			return ErrClusterDeploymentConflict
		}
		var source model.ClusterDeployment
		if err := tx.Where("id = ? AND server_id = ? AND operation = ? AND status = ? AND node_stopped = ? AND node_uninstalled = ?", removalTaskID, serverID, "remove", "success", true, false).First(&source).Error; err != nil {
			return err
		}
		if source.NodeMachineID == "" {
			return ErrClusterDeploymentConflict
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("(server_id = ? OR cluster_id = ?) AND status = ?", serverID, source.ClusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrClusterDeploymentConflict
		}
		task = source
		task.ID, task.Operation, task.Status, task.UninstallK3s = 0, "cleanup", "running", true
		task.CreatedAt, task.UpdatedAt = time.Time{}, time.Time{}
		return tx.Create(&task).Error
	})
	return &task, err
}

func (s *Store) FinishServerK3sCleanup(task *model.ClusterDeployment, success bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var persisted model.ClusterDeployment
		if err := tx.Where("id = ? AND operation = ? AND status = ?", task.ID, "cleanup", "running").First(&persisted).Error; err != nil {
			return err
		}
		status := "failed"
		if success {
			if !persisted.NodeUninstalled {
				return ErrClusterDeploymentConflict
			}
			status = "success"
			var source model.ClusterDeployment
			if err := tx.Where("id = (SELECT k3s_removal_task_id FROM servers WHERE id = ? AND deleted_at IS NULL) AND server_id = ? AND operation = ? AND status = ? AND node_uid = ? AND node_machine_id = ?", persisted.ServerID, persisted.ServerID, "remove", "success", persisted.NodeUID, persisted.NodeMachineID).First(&source).Error; err != nil {
				return err
			}
			result := tx.Model(&model.Server{}).Where("id = ? AND cluster_id IS NULL AND cluster_role = '' AND k8s_node_name = '' AND k3s_removal_task_id = ?", persisted.ServerID, source.ID).Update("k3s_removal_task_id", nil)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrClusterDeploymentConflict
			}
		}
		return tx.Model(&persisted).Update("status", status).Error
	})
}

func (s *Store) LatestServerK3sCleanup(serverID uint) (*model.ClusterDeployment, error) {
	var task model.ClusterDeployment
	err := s.db.Where("server_id = ? AND operation = ?", serverID, "cleanup").Order("id desc").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}

func (s *Store) ServerK3sCleanupSource(serverID uint) (*model.ClusterDeployment, error) {
	server, err := s.GetServer(serverID)
	if err != nil {
		return nil, err
	}
	if server.ClusterID != nil || server.ClusterRole != "" || server.K8sNodeName != "" || server.K3sRemovalTaskID == nil {
		return nil, ErrClusterDeploymentConflict
	}
	var source model.ClusterDeployment
	err = s.db.Where("id = ? AND server_id = ? AND operation = ? AND status = ? AND node_stopped = ? AND node_uninstalled = ?", *server.K3sRemovalTaskID, serverID, "remove", "success", true, false).First(&source).Error
	return &source, err
}
