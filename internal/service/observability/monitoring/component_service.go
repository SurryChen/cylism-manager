package monitoring

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
)

type ComponentAdapter interface {
	VictoriaMetricsStatusContext(context.Context) *k8s.VictoriaMetricsStatus
	InstallVictoriaMetricsContext(context.Context, k8s.VictoriaMetricsConfig) (*k8s.VictoriaMetricsStatus, error)
	UninstallVictoriaMetricsContext(context.Context) error
	StartVictoriaMetricsHostPathMigrationContext(context.Context, k8s.VictoriaMetricsMigrationRequest) (*k8s.VictoriaMetricsStatus, error)
}
type ComponentService struct{ adapter ComponentAdapter }

func NewComponentService(a ComponentAdapter) *ComponentService { return &ComponentService{adapter: a} }
func (s *ComponentService) Status(ctx context.Context) *k8s.VictoriaMetricsStatus {
	return s.adapter.VictoriaMetricsStatusContext(ctx)
}
func (s *ComponentService) Install(ctx context.Context, c k8s.VictoriaMetricsConfig) (*k8s.VictoriaMetricsStatus, error) {
	return s.adapter.InstallVictoriaMetricsContext(ctx, c)
}
func (s *ComponentService) Uninstall(ctx context.Context) error {
	return s.adapter.UninstallVictoriaMetricsContext(ctx)
}
func (s *ComponentService) Migrate(ctx context.Context, c k8s.VictoriaMetricsMigrationRequest) (*k8s.VictoriaMetricsStatus, error) {
	return s.adapter.StartVictoriaMetricsHostPathMigrationContext(ctx, c)
}
