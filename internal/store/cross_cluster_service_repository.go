package store

import (
	"errors"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

var ErrCrossClusterServiceConflict = errors.New("cross-cluster service name already exists")

func (s *Store) ListAllCrossClusterServices() ([]model.CrossClusterService, error) {
	items := make([]model.CrossClusterService, 0)
	err := s.db.Order("id desc").Find(&items).Error
	return items, err
}

func (s *Store) ListCrossClusterServices(clusterID uint) ([]model.CrossClusterService, error) {
	items := make([]model.CrossClusterService, 0)
	err := s.db.Where("source_cluster_id = ? OR target_cluster_id = ?", clusterID, clusterID).Order("id desc").Find(&items).Error
	return items, err
}

func (s *Store) GetCrossClusterService(clusterID, id uint) (*model.CrossClusterService, error) {
	var item model.CrossClusterService
	err := s.db.Where("id = ? AND (source_cluster_id = ? OR target_cluster_id = ?)", id, clusterID, clusterID).First(&item).Error
	return &item, err
}

func (s *Store) CreateCrossClusterService(item *model.CrossClusterService) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.CrossClusterService{}).Where("source_cluster_id = ? AND source_namespace = ? AND source_name = ?", item.SourceClusterID, item.SourceNamespace, item.SourceName).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCrossClusterServiceConflict
		}
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		if item.ID > 39999 {
			return errors.New("cross-cluster service port range exhausted")
		}
		item.GatewayPort = int32(20000 + item.ID)
		return tx.Model(item).Update("gateway_port", item.GatewayPort).Error
	})
}

func (s *Store) UpdateCrossClusterServiceState(id uint, status, message string) error {
	return s.db.Model(&model.CrossClusterService{}).Where("id = ?", id).Updates(map[string]any{"status": status, "last_error": message}).Error
}

func (s *Store) DeleteCrossClusterService(id uint) error {
	return s.db.Delete(&model.CrossClusterService{}, id).Error
}

func (s *Store) RecordCrossClusterServiceTask(clusterID, serverID uint, operation, message string, success bool) error {
	status := "success"
	if !success {
		status = "failed"
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		task := &model.ClusterDeployment{ClusterID: clusterID, ServerID: serverID, Operation: operation, Role: "service", Status: status}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Create(&model.ClusterDeploymentLog{DeploymentID: task.ID, Message: message}).Error
	})
}
