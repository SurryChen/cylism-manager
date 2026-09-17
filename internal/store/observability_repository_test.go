package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
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

func TestAuditLogPersistsStructuredEventFields(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	event := &model.AuditLog{
		Action:       "registry.mirror.verify",
		ResourceType: "node_registry_mirror",
		ResourceID:   5,
		TargetName:   "GHCR mirror",
		UserID:       2,
		ActorType:    model.AuditActorUser,
		ActorName:    "admin",
		Source:       model.AuditSourceAPI,
		Outcome:      model.AuditOutcomeSucceeded,
		Summary:      "验证节点镜像源 GHCR mirror",
		RequestID:    "req_123",
		OperationID:  "op_123",
		Detail:       `{"registry":"ghcr.io"}`,
	}
	if err := s.CreateAuditLog(event); err != nil {
		t.Fatalf("create audit event: %v", err)
	}
	var stored model.AuditLog
	if err := s.DB().First(&stored, event.ID).Error; err != nil {
		t.Fatalf("load audit event: %v", err)
	}
	if stored.ActorName != "admin" || stored.Outcome != model.AuditOutcomeSucceeded || stored.OperationID != "op_123" || stored.TargetName != "GHCR mirror" {
		t.Fatalf("structured audit fields were not persisted: %#v", stored)
	}
}

func TestListAuditLogsFilteredMatchesStructuredAndLegacyFields(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []*model.AuditLog{
		{Action: "application.release", ResourceType: "application", ResourceID: 1, TargetName: "console", ActorName: "admin", ActorType: model.AuditActorUser, Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded, Summary: "发布 console", Detail: `{"version":"1"}`},
		{Action: "application.release", ResourceType: "application", ResourceID: 2, TargetName: "worker", ActorName: "agent", ActorType: model.AuditActorAgent, Source: model.AuditSourceAgent, Outcome: model.AuditOutcomeFailed, Summary: "发布 worker", Detail: `{"reason":"failed"}`},
	} {
		if err := s.CreateAuditLog(event); err != nil {
			t.Fatal(err)
		}
	}
	logs, total, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Outcome: model.AuditOutcomeFailed, ActorType: model.AuditActorAgent, Keyword: "worker", Limit: 20})
	if err != nil || total != 1 || len(logs) != 1 || logs[0].TargetName != "worker" {
		t.Fatalf("unexpected filtered audit logs: %#v total=%d err=%v", logs, total, err)
	}
}
func TestSystemComponentConfigCRUD(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	config := &model.SystemComponentConfig{
		ChartName:     "coredns",
		Namespace:     "kube-system",
		ValuesContent: "replicas: 2\nmaxUnavailable: 0\n",
		Enabled:       true,
		ApplyStatus:   "succeeded",
		CreatedBy:     1,
	}
	if err := s.UpsertSystemComponentConfig(config); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if config.ID == 0 {
		t.Fatal("expected generated id")
	}
	loaded, err := s.GetSystemComponentConfig("coredns")
	if err != nil || loaded.ValuesContent != config.ValuesContent {
		t.Fatalf("unexpected loaded config: %#v err=%v", loaded, err)
	}
	config.ValuesContent = "replicas: 1\n"
	if err := s.UpsertSystemComponentConfig(config); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	loaded, _ = s.GetSystemComponentConfig("coredns")
	if loaded.ValuesContent != "replicas: 1\n" || loaded.ID != config.ID {
		t.Fatalf("update must keep identity and replace values: %#v", loaded)
	}
	all, err := s.ListSystemComponentConfigs()
	if err != nil || len(all) != 1 {
		t.Fatalf("unexpected list: %#v err=%v", all, err)
	}
	if err := s.DeleteSystemComponentConfig("coredns"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetSystemComponentConfig("coredns"); err != gorm.ErrRecordNotFound {
		t.Fatalf("expected record not found, got %v", err)
	}
}
