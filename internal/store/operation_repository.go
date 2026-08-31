package store

import (
	"github.com/cylism/cylism-manager/internal/model"
	"time"
)

func (s *Store) CreateOperationLog(log *model.OperationLog) error { return s.db.Create(log).Error }
func (s *Store) UpdateOperationLog(log *model.OperationLog) error { return s.db.Save(log).Error }
func (s *Store) ListOperationsByResource(resourceType string, resourceID uint) ([]model.OperationLog, error) {
	var logs []model.OperationLog
	err := s.db.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Order("created_at desc").Find(&logs).Error
	return logs, err
}
func (s *Store) DeleteExpiredOperationLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	threshold := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return s.db.Where("created_at < ?", threshold).Delete(&model.OperationLog{}).Error
}
