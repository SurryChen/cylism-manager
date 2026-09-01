package logging

import (
	"context"

	"github.com/cylism/cylism-manager/internal/k8s"
)

// ComponentAdapter is the narrow lifecycle boundary for the managed Loki and
// Alloy installation. Every remote operation receives the caller context.
type ComponentAdapter interface {
	LoggingStatus(context.Context) *k8s.LoggingStatus
	InstallLogging(context.Context, k8s.LoggingConfig) (*k8s.LoggingStatus, error)
	UninstallLogging(context.Context) error
}

type ComponentService struct{ adapter ComponentAdapter }

func NewComponentService(adapter ComponentAdapter) *ComponentService {
	return &ComponentService{adapter: adapter}
}

func (s *ComponentService) Status(ctx context.Context) *k8s.LoggingStatus {
	return s.adapter.LoggingStatus(ctx)
}

func (s *ComponentService) Install(ctx context.Context, config k8s.LoggingConfig) (*k8s.LoggingStatus, error) {
	return s.adapter.InstallLogging(ctx, config)
}

func (s *ComponentService) Uninstall(ctx context.Context) error {
	return s.adapter.UninstallLogging(ctx)
}
