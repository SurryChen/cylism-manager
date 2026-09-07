package alerting

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
)

type ComponentAdapter interface {
	AlertingStatusContext(context.Context) *k8s.AlertingStatus
	InstallAlertingContext(context.Context, k8s.AlertingConfig) (*k8s.AlertingStatus, error)
	UpdateAlertingContext(context.Context, k8s.AlertingConfig) (*k8s.AlertingStatus, error)
	UninstallAlertingContext(context.Context) error
}
type ComponentService struct{ adapter ComponentAdapter }

func NewComponentService(adapter ComponentAdapter) *ComponentService {
	return &ComponentService{adapter: adapter}
}
func (s *ComponentService) Status(ctx context.Context) *k8s.AlertingStatus {
	if s == nil || s.adapter == nil {
		return &k8s.AlertingStatus{}
	}
	return s.adapter.AlertingStatusContext(ctx)
}
func (s *ComponentService) Install(ctx context.Context, c k8s.AlertingConfig) (*k8s.AlertingStatus, error) {
	return s.adapter.InstallAlertingContext(ctx, c)
}
func (s *ComponentService) Update(ctx context.Context, c k8s.AlertingConfig) (*k8s.AlertingStatus, error) {
	return s.adapter.UpdateAlertingContext(ctx, c)
}
func (s *ComponentService) Uninstall(ctx context.Context) error {
	return s.adapter.UninstallAlertingContext(ctx)
}
