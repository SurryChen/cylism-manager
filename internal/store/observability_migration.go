package store

import (
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"time"
)

func (s *Store) CreateObservabilityMigration(item *model.ObservabilityMigration) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's write lock before reading the cluster task guard.
		result := tx.Model(&model.Cluster{}).Where("id = ?", item.ClusterID).UpdateColumn("id", item.ClusterID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("集群不存在")
		}
		var running int64
		if err := tx.Model(&model.ClusterDeployment{}).Where("cluster_id = ? AND status = ?", item.ClusterID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return ErrClusterDeploymentConflict
		}
		var target model.Server
		if err := tx.Where("cluster_id = ? AND k8s_node_name = ?", item.ClusterID, item.TargetNode).First(&target).Error; err != nil {
			return fmt.Errorf("目标节点必须绑定到当前集群的服务器")
		}
		task := model.ClusterDeployment{ClusterID: item.ClusterID, ServerID: target.ID, Operation: "migrate-" + item.Component, Role: item.Component, Status: "running"}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		item.TaskID = task.ID
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return tx.Create(&model.ClusterDeploymentLog{DeploymentID: task.ID, Message: item.Detail}).Error
	})
}

func (s *Store) SaveObservabilityMigration(item *model.ObservabilityMigration, message string, terminal bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Control requests and progress are written independently of stage changes.
		if err := tx.Omit("Control", "CanPause", "CanResume", "CanCancel").Save(item).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.ClusterDeploymentLog{DeploymentID: item.TaskID, Message: message}).Error; err != nil {
			return err
		}
		if terminal {
			status := "failed"
			if item.Status == "succeeded" {
				status = "success"
			}
			return tx.Model(&model.ClusterDeployment{}).Where("id = ?", item.TaskID).Update("status", status).Error
		}
		return nil
	})
}

func (s *Store) ListObservabilityMigrations(clusterID uint, component string) ([]model.ObservabilityMigration, error) {
	var items []model.ObservabilityMigration
	err := s.db.Where("cluster_id = ? AND component = ?", clusterID, component).Order("id DESC").Limit(20).Find(&items).Error
	return items, err
}

func (s *Store) ActiveObservabilityMigrations() ([]model.ObservabilityMigration, error) {
	var items []model.ObservabilityMigration
	err := s.db.Where("status NOT IN ?", []string{"succeeded", "failed", "rolled_back", "canceled"}).Find(&items).Error
	return items, err
}

func (s *Store) GetObservabilityMigration(id uint) (*model.ObservabilityMigration, error) {
	var item model.ObservabilityMigration
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) SetObservabilityMigrationControl(id uint, control, message string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ObservabilityMigration{}).Where("id = ?", id).Update("control", control).Error; err != nil {
			return err
		}
		var item model.ObservabilityMigration
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		return tx.Create(&model.ClusterDeploymentLog{DeploymentID: item.TaskID, Message: message}).Error
	})
}

func (s *Store) UpdateObservabilityMigrationProgress(id uint, total, copied, rate int64) error {
	now := time.Now()
	return s.db.Model(&model.ObservabilityMigration{}).Where("id = ? AND status IN ?", id, []string{"copying", "paused"}).Updates(map[string]interface{}{
		"bytes_total": total, "bytes_copied": copied, "bytes_per_second": rate, "progress_at": now,
	}).Error
}
