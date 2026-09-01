package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestBuildOverviewUsesStableJSONFieldNames(t *testing.T) {
	o := BuildOverview(nil, nil)
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"active":[],"resolved":[],"firing":0,"silenced":0}` {
		t.Fatalf("unexpected overview JSON: %s", raw)
	}
}

type workflowStoreFake struct {
	events []model.AlertEvent
}

func (f *workflowStoreFake) UpsertAlertEvent(event *model.AlertEvent) (*model.AlertEvent, error) {
	return event, nil
}
func (f *workflowStoreFake) UpdateAlertEvent(*model.AlertEvent) error { return nil }
func (f *workflowStoreFake) GetAlertAutomationPolicy() (*model.AlertAutomationPolicy, error) {
	return &model.AlertAutomationPolicy{}, nil
}
func (f *workflowStoreFake) SaveAlertAutomationPolicy(*model.AlertAutomationPolicy) error { return nil }
func (f *workflowStoreFake) ListAlertEvents(int) ([]model.AlertEvent, error)              { return f.events, nil }
func (f *workflowStoreFake) GetRuntime(uint) (*model.RuntimeInstance, error)              { return nil, nil }

func TestWorkflowOverviewDelegatesQueryAndResolvedCache(t *testing.T) {
	client := NewClient(func(_ context.Context, method, path string, _ interface{}, output interface{}) error {
		if method != http.MethodGet || path != "/api/v2/alerts" {
			t.Fatalf("unexpected request %s %s", method, path)
		}
		*(output.(*[]Alert)) = []Alert{{Status: AlertStatus{State: "active"}}}
		return nil
	})
	cache := NewResolvedCache(12)
	cache.Add([]Alert{{Fingerprint: "resolved-1", Status: AlertStatus{State: "resolved"}}})
	w := &Workflow{Client: client, Ready: func() bool { return true }, Cache: cache}
	overview, err := w.Overview(context.Background())
	if err != nil || overview.Firing != 1 || len(overview.Resolved) != 1 || overview.Resolved[0].Fingerprint != "resolved-1" {
		t.Fatalf("unexpected overview=%#v err=%v", overview, err)
	}
}

func TestWorkflowListEventsDelegatesToRepository(t *testing.T) {
	store := &workflowStoreFake{events: []model.AlertEvent{{ID: 3, AlertName: "NodeDown"}}}
	w := &Workflow{Store: store}
	events, err := w.ListEvents(30)
	if err != nil || len(events) != 1 || events[0].AlertName != "NodeDown" {
		t.Fatalf("unexpected events=%#v err=%v", events, err)
	}
}

func TestWorkflowRejectsInvalidNotification(t *testing.T) {
	w := &Workflow{}
	if err := w.ValidateNotification(AlertNotification{}); err == nil {
		t.Fatal("expected invalid notification error")
	}
}
