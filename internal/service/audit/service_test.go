package audit

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type fakeRepository struct {
	event *model.AuditLog
	err   error
}

func (r *fakeRepository) CreateAuditLog(event *model.AuditLog) error {
	r.event = event
	return r.err
}

func TestRecordWritesCompleteRedactedEvent(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	err := service.Record(AuditEventInput{
		Action:       "registry.mirror.verify",
		ResourceType: "node_registry_mirror",
		ResourceID:   1,
		TargetName:   "GHCR",
		Actor:        Actor{Type: model.AuditActorUser, ID: 2, Name: "admin"},
		Source:       model.AuditSourceAPI,
		Outcome:      model.AuditOutcomeSucceeded,
		Summary:      "验证节点镜像源 GHCR",
		RequestID:    "req_1",
		Metadata:     map[string]any{"token": "secret", "registry": "ghcr.io"},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if repo.event == nil || repo.event.Action != "registry.mirror.verify" || repo.event.ActorName != "admin" || repo.event.Outcome != model.AuditOutcomeSucceeded {
		t.Fatalf("incomplete audit event: %#v", repo.event)
	}
	if strings.Contains(repo.event.Detail, "secret") || !strings.Contains(repo.event.Detail, "[REDACTED]") {
		t.Fatalf("metadata leaked sensitive value: %s", repo.event.Detail)
	}
}

func TestRecordRejectsIncompleteEvent(t *testing.T) {
	service := NewService(&fakeRepository{})
	if err := service.Record(AuditEventInput{Action: "x"}); err == nil {
		t.Fatal("expected incomplete event to be rejected")
	}
}
