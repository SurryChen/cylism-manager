package authapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreauth "github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	authservice "github.com/cylism/cylism-manager/internal/service/auth"
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
	h := NewAuthHandlerWithTemporaryService(db, []byte("secret"), time.Hour, time.Hour, authservice.NewTemporaryTokenService(db, db))
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
}
