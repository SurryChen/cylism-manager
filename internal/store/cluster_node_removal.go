package store

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// StartClusterNodeRemoval reserves only a bound worker; identity is frozen for retries.
func (s *Store) StartClusterNodeRemoval(clusterID, serverID uint, identity *model.ClusterDeployment) (*model.ClusterDeployment, error) {
	if identity == nil {
		return nil, ErrClusterDeploymentConflict
	}
	task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: "remove", Role: "worker", Status: "running", NodeName: identity.NodeName, NodeUID: identity.NodeUID, NodeMachineID: identity.NodeMachineID, NodePodCIDRs: identity.NodePodCIDRs, NodeStopped: identity.NodeStopped}
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
		if success && !persisted.NodeStopped {
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
		result = tx.Model(&model.Server{}).Where("id = ? AND cluster_id = ? AND cluster_role = ? AND k8s_node_name = ?", persisted.ServerID, persisted.ClusterID, "worker", persisted.NodeName).Updates(map[string]any{"cluster_id": nil, "cluster_role": "", "k8s_node_name": ""})
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
