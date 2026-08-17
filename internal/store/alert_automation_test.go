package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAlertEventUpsertPreservesAutomationStateAndResolves(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	event, err := s.UpsertAlertEvent(&model.AlertEvent{Fingerprint: "fingerprint-1", AlertName: "NodeDiskHigh", Severity: "warning", NodeName: "node-a", Labels: `{}`, Annotations: `{}`, Status: model.AlertEventFiring, StartsAt: started})
	if err != nil || event.ID == 0 {
		t.Fatalf("create event: %#v %v", event, err)
	}
	event.Status, event.DiagnosticSummary = model.AlertEventAnalyzing, "分析中"
	if err := s.UpdateAlertEvent(event); err != nil {
		t.Fatal(err)
	}
	repeat, err := s.UpsertAlertEvent(&model.AlertEvent{Fingerprint: "fingerprint-1", AlertName: "NodeDiskHigh", Severity: "warning", NodeName: "node-a", Labels: `{"value":"new"}`, Annotations: `{}`, Status: model.AlertEventFiring, StartsAt: started})
	if err != nil || repeat.Status != model.AlertEventAnalyzing {
		t.Fatalf("repeat webhook must preserve automation status: %#v %v", repeat, err)
	}
	ended := time.Now().UTC()
	resolved, err := s.UpsertAlertEvent(&model.AlertEvent{Fingerprint: "fingerprint-1", AlertName: "NodeDiskHigh", Severity: "warning", NodeName: "node-a", Labels: `{}`, Annotations: `{}`, Status: model.AlertEventResolved, StartsAt: started, EndsAt: &ended})
	if err != nil || resolved.Status != model.AlertEventResolved || resolved.EndsAt == nil {
		t.Fatalf("resolve event: %#v %v", resolved, err)
	}
}

func TestAlertAutomationPolicyIsSingleton(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAlertAutomationPolicy(&model.AlertAutomationPolicy{RuntimeID: 7, Enabled: true, MinimumSeverity: "warning", Mode: model.AlertAutomationReportOnly, CooldownMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAlertAutomationPolicy(&model.AlertAutomationPolicy{RuntimeID: 8, Enabled: false, MinimumSeverity: "critical", Mode: model.AlertAutomationApproval, CooldownMinutes: 60}); err != nil {
		t.Fatal(err)
	}
	policy, err := s.GetAlertAutomationPolicy()
	if err != nil || policy.RuntimeID != 8 || policy.Enabled || policy.CooldownMinutes != 60 {
		t.Fatalf("unexpected singleton policy: %#v %v", policy, err)
	}
}
