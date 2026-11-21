package store

import (
	"context"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) ListAlertSuppressionRules(ctx context.Context, f model.AlertSuppressionFilter) ([]model.AlertSuppressionRule, int64, error) {
	q := s.db.WithContext(ctx).Model(&model.AlertSuppressionRule{}).Where("cluster_id = ?", f.ClusterID)
	if f.NodeName != "" {
		q = q.Where("node_name = ?", f.NodeName)
	}
	switch f.Status {
	case "active":
		q = q.Where("enabled = ? AND starts_at <= ? AND (ends_at IS NULL OR ends_at > ?)", true, f.Now, f.Now)
	case "disabled":
		q = q.Where("enabled = ?", false)
	case "expired":
		q = q.Where("enabled = ? AND ends_at <= ?", true, f.Now)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.AlertSuppressionRule, 0)
	limit, offset := historyPage(model.AlertHistoryFilter{Limit: f.Limit, Offset: f.Offset})
	err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (s *Store) ActiveAlertSuppressionRules(ctx context.Context, clusterID uint, now time.Time) ([]model.AlertSuppressionRule, error) {
	items := make([]model.AlertSuppressionRule, 0)
	err := s.db.WithContext(ctx).Where("cluster_id = ? AND enabled = ? AND starts_at <= ? AND (ends_at IS NULL OR ends_at > ?)", clusterID, true, now, now).Find(&items).Error
	return items, err
}

func (s *Store) CreateAlertSuppressionRule(ctx context.Context, rule *model.AlertSuppressionRule) error {
	return s.db.WithContext(ctx).Create(rule).Error
}

func (s *Store) UpdateAlertSuppressionRule(ctx context.Context, clusterID, id uint, rule *model.AlertSuppressionRule, resetDeadline bool) error {
	// An omitted duration preserves the stored deadline; toggles never extend it.
	fields := []string{"name", "node_name", "alert_names", "comment", "enabled", "updated_at"}
	if resetDeadline {
		fields = append(fields, "starts_at", "ends_at")
	}
	result := s.db.WithContext(ctx).Model(&model.AlertSuppressionRule{}).Where("cluster_id = ? AND id = ?", clusterID, id).
		Select(fields).Updates(rule)
	return suppressionMutationError(result)
}

func (s *Store) SetAlertSuppressionEnabled(ctx context.Context, clusterID, id uint, enabled bool) error {
	return suppressionMutationError(s.db.WithContext(ctx).Model(&model.AlertSuppressionRule{}).Where("cluster_id = ? AND id = ?", clusterID, id).Update("enabled", enabled))
}

func (s *Store) DeleteAlertSuppressionRule(ctx context.Context, clusterID, id uint) error {
	return suppressionMutationError(s.db.WithContext(ctx).Where("cluster_id = ? AND id = ?", clusterID, id).Delete(&model.AlertSuppressionRule{}))
}

func suppressionMutationError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
