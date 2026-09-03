package authapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreauth "github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestLoginRefreshAndMe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := coreauth.HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "alice", PasswordHash: hash}
	if err := db.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	h := NewAuthHandlerWithTemporaryService(db, []byte("secret"), time.Hour, 2*time.Hour, nil)
	r := gin.New()
	r.POST("/login", h.Login)
	r.POST("/refresh", h.Refresh)
	r.GET("/me", func(c *gin.Context) { c.Set("user_id", user.ID); c.Set("username", user.Username); h.Me(c) })

	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"username":"alice","password":"password"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(login, req)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", login.Code, login.Body.String())
	}
	var payload struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	claims, err := coreauth.ParseToken([]byte("secret"), payload.Data.AccessToken)
	if err != nil || claims.UserID != user.ID || claims.Username != user.Username {
		t.Fatalf("access claims = %#v, %v", claims, err)
	}

	refresh := httptest.NewRecorder()
	refreshReq := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBufferString(`{"refresh_token":"`+payload.Data.RefreshToken+`"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(refresh, refreshReq)
	if refresh.Code != http.StatusOK || !strings.Contains(refresh.Body.String(), "access_token") {
		t.Fatalf("refresh response = %d %s", refresh.Code, refresh.Body.String())
	}

	me := httptest.NewRecorder()
	r.ServeHTTP(me, httptest.NewRequest(http.MethodGet, "/me", nil))
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"username":"alice"`) {
		t.Fatalf("me response = %d %s", me.Code, me.Body.String())
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := coreauth.HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser(&model.User{Username: "alice", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	h := NewAuthHandlerWithTemporaryService(db, []byte("secret"), time.Hour, time.Hour, nil)
	r := gin.New()
	r.POST("/login", h.Login)
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"username":"alice","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d", resp.Code)
	}
}
