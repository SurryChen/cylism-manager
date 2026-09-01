package alerting

import (
	"context"
	"github.com/cylism/cylism-manager/internal/model"
	"time"
)

type EventStore interface {
	UpsertAlertEvent(*model.AlertEvent) (*model.AlertEvent, error)
	UpdateAlertEvent(*model.AlertEvent) error
	GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error)
	SaveAlertAutomationPolicy(*model.AlertAutomationPolicy) error
}
type Dispatcher interface {
	Dispatch(context.Context, *model.AlertEvent)
}

func PersistAndDispatch(ctx context.Context, store EventStore, dispatcher Dispatcher, events []*model.AlertEvent, force bool, now time.Time) (int, error) {
	if store == nil {
		return 0, nil
	}
	dispatched := 0
	policy, policyErr := store.GetAlertAutomationPolicy()
	for _, event := range events {
		persisted, err := store.UpsertAlertEvent(event)
		if err != nil {
			return dispatched, err
		}
		if policyErr != nil || dispatcher == nil || persisted.Status == model.AlertEventResolved || !ShouldDispatch(policy, persisted, force, now) {
			continue
		}
		persisted.Status = model.AlertEventAnalyzing
		dispatchedAt := now
		persisted.LastDispatchedAt = &dispatchedAt
		if err := store.UpdateAlertEvent(persisted); err != nil {
			return dispatched, err
		}
		dispatched++
		go dispatcher.Dispatch(ctx, persisted)
	}
	return dispatched, nil
}
