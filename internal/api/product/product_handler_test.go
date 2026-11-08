package productapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestProductAPIValidationAndIsolation(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "other" {
			c.Set("user_id", uint(2))
		} else {
			c.Set("user_id", uint(1))
		}
	})
	r.POST("/products", h.Create)
	r.GET("/products/:id", h.Get)
	r.PUT("/products/:id", h.Update)
	request := func(method, path, body, user string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	for _, body := range []string{
		`{"name":"  "}`, `{"name":"x","stage":"nonsense"}`, `{"name":"x","public_url":"javascript:alert(1)"}`,
		`{"name":"x","next_due_date":"2026-02-30"}`, `{"name":"x","next_due_date":"2026-11-09"}`,
		`{"name":"x","channels":[{"platform":"web","publish_status":"nonsense"}]}`,
		`{"name":"x","links":[{"kind":"repository","url":"file:///etc/passwd"}]}`,
	} {
		res := request("POST", "/products", body, "")
		if res.Code != 400 {
			t.Fatalf("expected validation failure for %s: %d %s", body, res.Code, res.Body.String())
		}
	}
	res := request("POST", "/products", `{"name":"Offline","next_action":"Ship","next_due_date":"2026-11-09","public_showcase":true,"channels":[{"platform":"google_play","publish_status":"in_review"}]}`, "")
	if res.Code != 200 {
		t.Fatalf("create: %d %s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data model.Product `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID != 1 || envelope.Data.OwnerID != 1 || envelope.Data.Stage != "idea" || envelope.Data.NextDueDate == nil {
		t.Fatalf("bad product: %#v", envelope.Data)
	}
	if got := request("GET", "/products/1", "", "other"); got.Code != 404 {
		t.Fatalf("other owner read: %d", got.Code)
	}
	if got := request("PUT", "/products/1", `{"name":"stolen"}`, "other"); got.Code != 404 {
		t.Fatalf("other owner mutation: %d", got.Code)
	}
	if got := request("GET", "/products/0", "", ""); got.Code != 400 {
		t.Fatalf("zero id: %d", got.Code)
	}
}
