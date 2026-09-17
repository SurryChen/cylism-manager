package authapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	authservice "github.com/cylism/cylism-manager/internal/service/auth"
	coreauth "github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestTemporaryLoginIssuesNormalSessionAndCanBeRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "token-owner", PasswordHash: "hash"}
	if err := db.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	h := NewAuthHandlerWithTemporaryService(db, []byte("secret"), time.Hour, time.Hour, authservice.NewTemporaryTokenService(db, db)).WithAudit(db)
	r := gin.New()
	r.POST("/create", func(c *gin.Context) { c.Set("user_id", user.ID); h.CreateTemporaryToken(c) })
	r.POST("/login", h.TemporaryLogin)

	createReq := httptest.NewRequest(http.MethodPost, "/create", bytes.NewBufferString(`{"label":"test","ttl_seconds":3600}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create status = %d, body=%s", createResp.Code, createResp.Body.String())
	}
	var envelope struct {
		Data struct {
			ID    uint   `json:"id"`
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == 0 || envelope.Data.Token == "" {
		t.Fatalf("unexpected create response: %s", createResp.Body.String())
	}

	loginBody := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"token":"`+envelope.Data.Token+`"}`))
	loginBody.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	r.ServeHTTP(loginResp, loginBody)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", loginResp.Code, loginResp.Body.String())
	}
	var loginEnvelope struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginResp.Body.Bytes(), &loginEnvelope); err != nil {
		t.Fatal(err)
	}
	claims, err := coreauth.ParseToken([]byte("secret"), loginEnvelope.Data.AccessToken)
	if err != nil || claims.UserID != user.ID {
		t.Fatalf("claims = %#v, err=%v", claims, err)
	}
	events, total, err := db.ListAuditLogsFiltered(model.AuditLogFilter{Limit: 20})
	if err != nil || total != 2 {
		t.Fatalf("expected temporary token grant and exchange audits: %#v total=%d err=%v", events, total, err)
	}
	actions := map[string]bool{}
	for _, event := range events {
		if strings.Contains(event.Detail, envelope.Data.Token) {
			t.Fatalf("temporary token leaked into audit detail: %#v", event)
		}
		actions[event.Action] = true
	}
	if !actions["auth.temporary_token.create"] || !actions["auth.temporary_login"] {
		t.Fatalf("missing temporary token audit actions: %#v", actions)
	}
}
