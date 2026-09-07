package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) GetDashboardStats(daysBefore int) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{}
	s.db.Model(&model.Server{}).Count(&stats.TotalServers)
	s.db.Model(&model.Site{}).Count(&stats.TotalSites)
	threshold := time.Now().Add(time.Duration(daysBefore) * 24 * time.Hour)
	s.db.Model(&model.Cert{}).Where("status = ? AND valid_to <= ?", "issued", threshold).Count(&stats.ExpiringCerts)
	return stats, nil
}

func (s *Store) UpsertAlertEvent(event *model.AlertEvent) (*model.AlertEvent, error) {
	if event == nil || event.Fingerprint == "" || event.AlertName == "" || event.Labels == "" || event.Annotations == "" || event.Status == "" || event.StartsAt.IsZero() {
		return nil, errors.New("invalid alert event")
	}
	var stored model.AlertEvent
	err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("fingerprint = ?", event.Fingerprint).First(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(event).Error
		}
		if err != nil {
			return err
		}
		updates := map[string]interface{}{
			"alert_name": event.AlertName, "severity": event.Severity, "node_name": event.NodeName,
			"mount_point": event.MountPoint, "labels": event.Labels, "annotations": event.Annotations,
			"starts_at": event.StartsAt,
		}
		if event.Status == model.AlertEventResolved {
			updates["status"] = event.Status
			updates["ends_at"] = event.EndsAt
		} else if stored.Status == model.AlertEventResolved {
			updates["status"] = model.AlertEventFiring
			updates["ends_at"] = nil
		}
		if err := tx.Model(&stored).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("fingerprint = ?", event.Fingerprint).First(&stored).Error
	})
	if err != nil {
		return nil, err
	}
	if event.ID != 0 {
		return event, nil
	}
	return &stored, nil
}

func (s *Store) GetAlertEvent(id uint) (*model.AlertEvent, error) {
	var event model.AlertEvent
	err := s.db.First(&event, id).Error
	return &event, err
}

func (s *Store) ListAlertEvents(limit int) ([]model.AlertEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	var events []model.AlertEvent
	err := s.db.Order("updated_at desc").Limit(limit).Find(&events).Error
	return events, err
}

func (s *Store) UpdateAlertEvent(event *model.AlertEvent) error {
	if event == nil || event.ID == 0 {
		return errors.New("invalid alert event")
	}
	return s.db.Save(event).Error
}

func (s *Store) GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error) {
	var policy model.AlertAutomationPolicy
	err := s.db.Order("id asc").First(&policy).Error
	return &policy, err
}

func (s *Store) SaveAlertAutomationPolicy(policy *model.AlertAutomationPolicy) error {
	if policy == nil {
		return errors.New("invalid alert automation policy")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.AlertAutomationPolicy
		err := tx.Order("id asc").First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(policy).Error
		}
		if err != nil {
			return err
		}
		policy.ID, policy.CreatedAt = existing.ID, existing.CreatedAt
		return tx.Save(policy).Error
	})
}

func (s *Store) CreateAuditLog(entry *model.AuditLog) error {
	return s.db.Create(entry).Error
}

func (s *Store) ListAuditLogs(resourceType, action string, limit, offset int) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64
	query := s.db.Model(&model.AuditLog{})
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	query.Count(&total)
	err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

func (s *Store) ListSystemComponentConfigs() ([]model.SystemComponentConfig, error) {
	var configs []model.SystemComponentConfig
	err := s.db.Order("chart_name asc").Find(&configs).Error
	return configs, err
}

func (s *Store) GetSystemComponentConfig(chartName string) (*model.SystemComponentConfig, error) {
	var config model.SystemComponentConfig
	err := s.db.Where("chart_name = ?", chartName).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) UpsertSystemComponentConfig(config *model.SystemComponentConfig) error {
	var existing model.SystemComponentConfig
	err := s.db.Where("chart_name = ?", config.ChartName).First(&existing).Error
	if err == nil {
		config.ID = existing.ID
		config.CreatedAt = existing.CreatedAt
		return s.db.Save(config).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(config).Error
}

func (s *Store) DeleteSystemComponentConfig(chartName string) error {
	return s.db.Where("chart_name = ?", chartName).Delete(&model.SystemComponentConfig{}).Error
}

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
	threshold := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("created_at < ?", threshold).Delete(&model.OperationLog{}).Error
}
