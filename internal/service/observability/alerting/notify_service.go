package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type AlertNotification struct {
	Status      string
	Alerts      []Alert
	PlatformURL string
}

type EventWorkflow struct {
	Automation *AutomationService
	Store      EventStore
	Dispatcher Dispatcher
	Now        func() time.Time
}

func (w EventWorkflow) Persist(ctx context.Context, payload AlertNotification, force bool) (int, error) {
	events := make([]*model.AlertEvent, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		events = append(events, EventFromAlert(alert, strings.EqualFold(payload.Status, "resolved")))
	}
	if w.Automation != nil {
		return w.Automation.PersistAndDispatch(ctx, events, force)
	}
	if w.Store == nil {
		return 0, nil
	}
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	return PersistAndDispatch(ctx, w.Store, w.Dispatcher, events, force, now)
}

func (w EventWorkflow) Sync(ctx context.Context, alerts []Alert) (int, error) {
	firing := make([]Alert, 0, len(alerts))
	for _, alert := range alerts {
		if IsFiring(alert) {
			firing = append(firing, alert)
		}
	}
	if len(firing) == 0 {
		return 0, nil
	}
	return w.Persist(ctx, AlertNotification{Status: "firing", Alerts: firing}, true)
}

func ValidateRelayToken(header, expected string) bool {
	parts := strings.Fields(header)
	return len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && expected != "" && parts[1] == expected
}

var ErrInvalidNotification = fmt.Errorf("告警回调载荷无效")
