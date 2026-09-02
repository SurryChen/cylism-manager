package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// AutomationRepository is the durable state required by alert automation.
// It is deliberately split from the HTTP/Kubernetes adapters so the workflow
// can be exercised with a small fake in unit tests.
type AutomationRepository interface {
	EventStore
	GetRuntime(uint) (*model.RuntimeInstance, error)
}

type AutomationService struct {
	repo       AutomationRepository
	dispatcher Dispatcher
	now        func() time.Time
}

// SyncActiveAlerts imports the current firing set and forces one automation
// evaluation pass without sending a second external notification.
func (s *AutomationService) SyncActiveAlerts(ctx context.Context, read func(context.Context) ([]*model.AlertEvent, error)) (int, error) {
	if s == nil || read == nil {
		return 0, fmt.Errorf("活跃告警读取不可用")
	}
	events, err := read(ctx)
	if err != nil {
		return 0, err
	}
	return s.PersistAndDispatch(ctx, events, true)
}

func NewAutomationService(repo AutomationRepository, dispatcher Dispatcher) *AutomationService {
	return &AutomationService{repo: repo, dispatcher: dispatcher, now: time.Now}
}

// WithDispatcher attaches the runtime dispatch boundary after the service has
// been composed. This keeps construction in Bootstrap while allowing the API
// package to provide its transport-specific dispatcher.
func (s *AutomationService) WithDispatcher(dispatcher Dispatcher) *AutomationService {
	if s != nil {
		s.dispatcher = dispatcher
	}
	return s
}

func (s *AutomationService) Policy() (*model.AlertAutomationPolicy, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("告警自动化不可用")
	}
	return s.repo.GetAlertAutomationPolicy()
}

func (s *AutomationService) UpdatePolicy(ctx context.Context, policy *model.AlertAutomationPolicy, sync func(context.Context) (int, error)) (int, error) {
	if s == nil || s.repo == nil {
		return 0, fmt.Errorf("告警自动化不可用")
	}
	if !ValidPolicy(policy) || policy.RuntimeID == 0 {
		return 0, fmt.Errorf("告警自动化策略无效")
	}
	runtime, err := s.repo.GetRuntime(policy.RuntimeID)
	if err != nil || runtime == nil || runtime.RuntimeType != model.RuntimeTypeNanobot || runtime.DeploymentMode != model.RuntimeDeploymentManaged {
		return 0, fmt.Errorf("必须选择已托管的 Nanobot Runtime")
	}
	if err := s.repo.SaveAlertAutomationPolicy(policy); err != nil {
		return 0, err
	}
	if !policy.Enabled || sync == nil {
		return 0, nil
	}
	return sync(ctx)
}

func (s *AutomationService) PersistAndDispatch(ctx context.Context, events []*model.AlertEvent, force bool) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("告警自动化不可用")
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	return PersistAndDispatch(ctx, s.repo, s.dispatcher, events, force, now)
}
