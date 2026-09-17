// Package audit provides the typed boundary for durable audit events.
package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	security "github.com/cylism/cylism-manager/internal/security"
)

type Repository interface {
	CreateAuditLog(*model.AuditLog) error
}

type Actor struct {
	Type string
	ID   uint
	Name string
}

type AuditEventInput struct {
	Action       string
	ResourceType string
	ResourceID   uint
	TargetName   string
	Actor        Actor
	Source       string
	Outcome      string
	Summary      string
	RequestID    string
	OperationID  string
	Metadata     map[string]any
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Record(input AuditEventInput) error {
	if s == nil || s.repository == nil {
		return errors.New("audit repository is unavailable")
	}
	if strings.TrimSpace(input.Action) == "" || strings.TrimSpace(input.ResourceType) == "" || strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Outcome) == "" || strings.TrimSpace(input.Summary) == "" {
		return errors.New("incomplete audit event")
	}
	if input.TargetName == "" {
		input.TargetName = model.AuditTargetFallback(input.ResourceType, input.ResourceID)
	}
	if input.Actor.Type == "" {
		input.Actor.Type = model.AuditActorSystem
	}
	metadata, err := json.Marshal(redactMetadata(input.Metadata))
	if err != nil {
		return err
	}
	return s.repository.CreateAuditLog(&model.AuditLog{
		Action:       input.Action,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		TargetName:   input.TargetName,
		UserID:       input.Actor.ID,
		ActorType:    input.Actor.Type,
		ActorName:    input.Actor.Name,
		Source:       input.Source,
		Outcome:      input.Outcome,
		Summary:      input.Summary,
		RequestID:    input.RequestID,
		OperationID:  input.OperationID,
		Detail:       string(metadata),
		CreatedAt:    time.Now().UTC(),
	})
}

func redactMetadata(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactMetadata(item)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, item := range typed {
			redacted[index] = redactMetadata(item)
		}
		return redacted
	case string:
		return security.Redact(typed)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "private_key") || key == "content"
}
