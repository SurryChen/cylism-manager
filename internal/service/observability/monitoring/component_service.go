package monitoring

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
)

type ComponentAdapter interface {
	VictoriaMetricsStatus(context.Context) *k8s.VictoriaMetricsStatus
	InstallVictoriaMetrics(context.Context, k8s.VictoriaMetricsConfig) (*k8s.VictoriaMetricsStatus, error)
	UninstallVictoriaMetrics(context.Context) error
	StartVictoriaMetricsHostPathMigration(context.Context, k8s.VictoriaMetricsMigrationRequest) (*k8s.VictoriaMetricsStatus, error)
}
type ComponentService struct{ adapter ComponentAdapter }

func NewComponentService(a ComponentAdapter) *ComponentService { return &ComponentService{adapter: a} }
func (s *ComponentService) Status(ctx context.Context) *k8s.VictoriaMetricsStatus {
	return s.adapter.VictoriaMetricsStatus(ctx)
}
func (s *ComponentService) Install(ctx context.Context, c k8s.VictoriaMetricsConfig) (*k8s.VictoriaMetricsStatus, error) {
	return s.adapter.InstallVictoriaMetrics(ctx, c)
}
func (s *ComponentService) Uninstall(ctx context.Context) error {
	return s.adapter.UninstallVictoriaMetrics(ctx)
}
func (s *ComponentService) Migrate(ctx context.Context, c k8s.VictoriaMetricsMigrationRequest) (*k8s.VictoriaMetricsStatus, error) {
	return s.adapter.StartVictoriaMetricsHostPathMigration(ctx, c)
}
