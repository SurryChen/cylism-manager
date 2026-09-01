package alerting

import (
	"context"
	"github.com/cylism/cylism-manager/internal/k8s"
)

type ComponentAdapter interface {
	AlertingStatus(context.Context) *k8s.AlertingStatus
	InstallAlerting(context.Context, k8s.AlertingConfig) (*k8s.AlertingStatus, error)
	UpdateAlerting(context.Context, k8s.AlertingConfig) (*k8s.AlertingStatus, error)
	UninstallAlerting(context.Context) error
}
type ComponentService struct{ adapter ComponentAdapter }

func NewComponentService(adapter ComponentAdapter) *ComponentService {
	return &ComponentService{adapter: adapter}
}
func (s *ComponentService) Status(ctx context.Context) *k8s.AlertingStatus {
	if s == nil || s.adapter == nil {
		return &k8s.AlertingStatus{}
	}
	return s.adapter.AlertingStatus(ctx)
}
func (s *ComponentService) Install(ctx context.Context, c k8s.AlertingConfig) (*k8s.AlertingStatus, error) {
	return s.adapter.InstallAlerting(ctx, c)
}
func (s *ComponentService) Update(ctx context.Context, c k8s.AlertingConfig) (*k8s.AlertingStatus, error) {
	return s.adapter.UpdateAlerting(ctx, c)
}
func (s *ComponentService) Uninstall(ctx context.Context) error {
	return s.adapter.UninstallAlerting(ctx)
}
