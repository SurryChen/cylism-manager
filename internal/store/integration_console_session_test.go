package store

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestIntegrationSessionConsumesHandoffOnce(t *testing.T) {
	s := setupTestDB(t)
	now := time.Now()
	for _, handoff := range []string{"handoff-a", "handoff-b"} {
		if err := s.CreateIntegrationSession(&model.IntegrationSession{HandoffCodeHash: handoff, UserID: 1, ProjectID: 2, ApplicationID: 3, EnvironmentID: 4, ActionsData: "application:read", HandoffExpiresAt: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatalf("create %s: %v", handoff, err)
		}
	}
	if _, err := s.ExchangeIntegrationSession("handoff-a", "session-a", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeIntegrationSession("handoff-a", "session-b", now.Add(time.Hour), now); err == nil {
		t.Fatal("used handoff code was accepted")
	}
	if session, err := s.GetActiveIntegrationSession("session-a", now); err != nil || session.ApplicationID != 3 {
		t.Fatalf("active session = %#v, err = %v", session, err)
	}
}
