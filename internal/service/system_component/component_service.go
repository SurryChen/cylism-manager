package system_component

import (
	"context"
	"errors"
	"time"
)

var errUnavailable = errors.New("系统组件服务不可用")

// ComponentService is the application-facing system-component workflow. It
// owns the list, update, revert and reconcile boundaries so HTTP handlers do
// not assemble service objects or call package-level workflows directly.
type ComponentService struct {
	configs ConfigRepository
	adapter KubernetesAdapter
	list    *ComponentListService
	now     func() time.Time
}

func NewComponentService(configs ConfigRepository, adapter KubernetesAdapter, list *ComponentListService) *ComponentService {
	if list == nil {
		if repo, ok := configs.(ListRepository); ok {
			if a, ok := adapter.(interface {
				ListAdapter
				Available() bool
			}); ok {
				list = &ComponentListService{Repo: repo, Adapter: a}
			}
		}
	}
	return &ComponentService{configs: configs, adapter: adapter, list: list, now: time.Now}
}

func (s *ComponentService) List(ctx context.Context) ([]map[string]interface{}, error) {
	if s == nil || s.list == nil {
		return nil, errUnavailable
	}
	return s.list.List(ctx)
}

func (s *ComponentService) Update(ctx context.Context, chart, values, timeout string, userID uint) (*UpdateResult, error) {
	if s == nil {
		return nil, errUnavailable
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	repo, ok := s.configs.(UpdateRepository)
	if !ok {
		return nil, errUnavailable
	}
	return Update(ctx, repo, s.adapter, chart, values, timeout, userID, now)
}

func (s *ComponentService) Revert(ctx context.Context, chart string) error {
	if s == nil {
		return context.Canceled
	}
	repo, ok := s.configs.(RevertRepository)
	if !ok {
		return context.Canceled
	}
	return RevertManaged(ctx, repo, s.adapter, chart)
}

func (s *ComponentService) Run(ctx context.Context, interval time.Duration) error {
	if s == nil {
		return nil
	}
	return Run(ctx, interval, s.configs, s.adapter, ParseStaticDeploymentConfig, func(ctx context.Context, node string) error {
		return ValidateNode(ctx, s.adapter, node)
	}, time.Now)
}
