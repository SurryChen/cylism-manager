package store

import (
	"context"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) RecordAlertHistory(ctx context.Context, records []model.AlertHistory) error {
	if len(records) == 0 {
		return nil
	}
	// Once resolved, late retries of the firing callback cannot reopen this
	// occurrence. A subsequent trigger has a different starts_at and a new row.
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "cluster_id"}, {Name: "fingerprint"}, {Name: "starts_at"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"rule_name": gorm.Expr("excluded.rule_name"), "severity": gorm.Expr("excluded.severity"),
			"summary": gorm.Expr("excluded.summary"), "annotations": gorm.Expr("excluded.annotations"),
			"notification_suppressed": gorm.Expr("excluded.notification_suppressed"),
			"status":                  gorm.Expr("CASE WHEN excluded.status = 'resolved' THEN 'resolved' ELSE alert_histories.status END"),
			"ends_at":                 gorm.Expr("COALESCE(alert_histories.ends_at, excluded.ends_at)"),
			"updated_at":              gorm.Expr("excluded.updated_at"),
		}),
	}).Create(&records).Error
}

func (s *Store) alertHistoryQuery(ctx context.Context, f model.AlertHistoryFilter) *gorm.DB {
	q := s.db.WithContext(ctx).Model(&model.AlertHistory{}).Where("cluster_id = ?", f.ClusterID)
	if !f.Since.IsZero() {
		q = q.Where("starts_at >= ?", f.Since)
	}
	if node := strings.TrimSpace(f.NodeName); node != "" {
		q = q.Where("node_name = ?", node)
	}
	if name := strings.TrimSpace(f.AlertName); name != "" {
		q = q.Where("alert_name = ?", name)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	return q
}

func historyPage(f model.AlertHistoryFilter) (int, int) {
	limit, offset := f.Limit, f.Offset
	if limit < 1 || limit > 100 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Store) ListAlertHistory(ctx context.Context, f model.AlertHistoryFilter) ([]model.AlertHistory, int64, error) {
	q := s.alertHistoryQuery(ctx, f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.AlertHistory, 0)
	limit, offset := historyPage(f)
	err := q.Order("starts_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (s *Store) ListAlertFrequency(ctx context.Context, f model.AlertHistoryFilter) ([]model.AlertFrequency, int64, error) {
	groups := s.alertHistoryQuery(ctx, f).Select("node_name, alert_name, MAX(rule_name) AS rule_name, COUNT(*) AS count, strftime('%Y-%m-%dT%H:%M:%fZ', MAX(starts_at)) AS last_seen").Group("node_name, alert_name")
	var total int64
	if err := s.db.WithContext(ctx).Table("(?) AS frequencies", groups).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.AlertFrequency, 0)
	limit, offset := historyPage(f)
	err := s.db.WithContext(ctx).Table("(?) AS frequencies", groups).Order("count DESC, last_seen DESC, node_name ASC, alert_name ASC").Limit(limit).Offset(offset).Scan(&items).Error
	return items, total, err
}
