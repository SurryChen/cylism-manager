package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

type orchestratorStoreFake struct {
	policy *model.AlertAutomationPolicy
}

func (f *orchestratorStoreFake) UpsertAlertEvent(event *model.AlertEvent) (*model.AlertEvent, error) {
	return event, nil
}
func (f *orchestratorStoreFake) UpdateAlertEvent(*model.AlertEvent) error { return nil }
func (f *orchestratorStoreFake) GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error) {
	return f.policy, nil
}
func (f *orchestratorStoreFake) SaveAlertAutomationPolicy(*model.AlertAutomationPolicy) error {
	return nil
}

type orchestratorDispatcherFunc func(context.Context, *model.AlertEvent)

func (f orchestratorDispatcherFunc) Dispatch(ctx context.Context, event *model.AlertEvent) {
	f(ctx, event)
}

func TestPersistAndDispatchSurvivesRequestCancellation(t *testing.T) {
	store := &orchestratorStoreFake{policy: &model.AlertAutomationPolicy{
		Enabled:         true,
		MinimumSeverity: "warning",
		CooldownMinutes: 30,
		Mode:            model.AlertAutomationReportOnly,
	}}
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	observedErr := make(chan error, 1)

	count, err := PersistAndDispatch(requestCtx, store, orchestratorDispatcherFunc(func(ctx context.Context, _ *model.AlertEvent) {
		observedErr <- ctx.Err()
	}), []*model.AlertEvent{{
		Fingerprint: "fingerprint-1",
		AlertName:   "NodeDown",
		Severity:    "warning",
		Status:      model.AlertEventFiring,
	}}, false, time.Now())
	if err != nil || count != 1 {
		t.Fatalf("unexpected dispatch result: count=%d err=%v", count, err)
	}

	select {
	case err := <-observedErr:
		if err != nil {
			t.Fatalf("background dispatch context was canceled: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch did not start")
	}
}
