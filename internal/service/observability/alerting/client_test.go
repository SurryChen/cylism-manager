package alerting

import (
	"context"
	"github.com/cylism/cylism-manager/internal/model"
	"testing"
	"time"
)

func TestClientDelegatesRequest(t *testing.T) {
	called := false
	c := NewClient(func(_ context.Context, method, path string, _, _ interface{}) error {
		called = method == "GET" && path == "/api/v1/alerts"
		return nil
	})
	if err := c.Request(context.Background(), "GET", "/api/v1/alerts", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("request was not delegated")
	}
}

func TestShouldDispatchHonorsCooldownAndSeverity(t *testing.T) {
	now := time.Now()
	policy := &model.AlertAutomationPolicy{Enabled: true, MinimumSeverity: "warning", Mode: model.AlertAutomationReportOnly, CooldownMinutes: 30}
	event := &model.AlertEvent{AlertName: "disk", Severity: "critical"}
	if !ShouldDispatch(policy, event, false, now) {
		t.Fatal("first event should dispatch")
	}
	event.LastDispatchedAt = ptrTime(now.Add(-10 * time.Minute))
	if ShouldDispatch(policy, event, false, now) {
		t.Fatal("cooldown should suppress dispatch")
	}
	if !ShouldDispatch(policy, event, true, now) {
		t.Fatal("forced dispatch should bypass cooldown")
	}
}
func ptrTime(v time.Time) *time.Time { return &v }

func TestSendAggregatesChannelFailures(t *testing.T) {
	err := Send(context.Background(), Notification{FeishuWebhook: "https://open.feishu.cn/x", EmailEnabled: true}, "payload",
		func(context.Context, string, interface{}) error { return context.DeadlineExceeded },
		func(context.Context, interface{}, interface{}) error { return context.Canceled })
	if err == nil {
		t.Fatal("expected aggregated notification error")
	}
}
