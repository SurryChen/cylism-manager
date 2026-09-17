package applicationapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestCreateIntegrationDelegationAuditsDelegatedIssuance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sessionToken := "active-integration-session"
	handler := newTestApplicationHandler(db, []byte("01234567890123456789012345678901"), nil).
		WithDelegationSecret([]byte("delegation-secret")).WithAudit(db)
	if err := db.CreateIntegrationSession(&model.IntegrationSession{
		HandoffCodeHash: "handoff-hash", SessionTokenHash: stringPointer(opaqueHash(sessionToken)),
		UserID: 7, ProjectID: 3, ApplicationID: 11, EnvironmentID: 5, Capability: "reader",
		ActionsData: "application:read,configmap:write", HandoffExpiresAt: time.Now().Add(time.Hour), ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/delegation", handler.CreateIntegrationDelegation)
	request := newJSONRequest(http.MethodPost, "/delegation", map[string]string{"capability": "reader"})
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	response := serve(router, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delegation status=%d body=%s", response.Code, response.Body.String())
	}
	events, total, err := db.ListAuditLogsFiltered(model.AuditLogFilter{Action: "application.delegation.create", Source: model.AuditSourceDelegation, Limit: 20})
	if err != nil || total != 1 || len(events) != 1 || events[0].ResourceID != 11 || events[0].UserID != 7 {
		t.Fatalf("unexpected delegated issuance audit: %#v total=%d err=%v", events, total, err)
	}
}

func stringPointer(value string) *string { return &value }
