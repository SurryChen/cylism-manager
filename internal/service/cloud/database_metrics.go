package cloud

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

type DatabaseMetricPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}
type DatabaseMetricSeries struct {
	Key    string                `json:"key"`
	Label  string                `json:"label"`
	Unit   string                `json:"unit"`
	Values []DatabaseMetricPoint `json:"values"`
}
type DatabaseMetrics struct {
	InstanceID  string                 `json:"instance_id"`
	CollectedAt time.Time              `json:"collected_at"`
	Series      []DatabaseMetricSeries `json:"series"`
}
type DatabaseMetricsProvider interface {
	DatabaseMetrics(context.Context, string, string, time.Time, time.Time) (DatabaseMetrics, error)
}

var databaseRegionPattern = regexp.MustCompile(`^[a-z]{2}-[a-z0-9]+(?:-[a-z0-9]+)*$`)
var cloudDatabaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{3,128}$`)

func (s *Service) ValidateDatabaseMonitor(id uint, region, instance string) error {
	if !databaseRegionPattern.MatchString(region) || !cloudDatabaseIDPattern.MatchString(instance) {
		return fmt.Errorf("请填写有效的地域和云数据库实例 ID")
	}
	connection, err := s.GetConnection(id)
	if err != nil {
		return fmt.Errorf("云账号不存在")
	}
	if connection.Provider != "aliyun" {
		return fmt.Errorf("当前仅支持阿里云 RDS 原生监控")
	}
	return nil
}

func (s *Service) DatabaseMetrics(ctx context.Context, id uint, region, instance string, hours int) (DatabaseMetrics, error) {
	if err := s.ValidateDatabaseMonitor(id, region, instance); err != nil {
		return DatabaseMetrics{}, err
	}
	if hours != 1 && hours != 6 && hours != 24 {
		return DatabaseMetrics{}, fmt.Errorf("时间范围应为 1、6 或 24 小时")
	}
	_, provider, err := s.provider(id)
	if err != nil {
		return DatabaseMetrics{}, err
	}
	p, ok := provider.(DatabaseMetricsProvider)
	if !ok {
		return DatabaseMetrics{}, fmt.Errorf("该云账号不支持数据库监控")
	}
	end := time.Now().UTC()
	return p.DatabaseMetrics(ctx, region, instance, end.Add(-time.Duration(hours)*time.Hour), end)
}
