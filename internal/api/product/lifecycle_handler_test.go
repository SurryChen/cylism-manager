package productapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductLifecycleAPIContracts(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	p := model.Product{OwnerID: 1, Name: "offline", Stage: "developing", Channels: []model.ProductChannel{{Platform: "google_play", PublishStatus: "draft"}}}
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-User") == "owner" {
			c.Set("user_id", uint(1))
		} else if c.GetHeader("X-User") == "other" {
			c.Set("user_id", uint(2))
		}
	})
	r.GET("/products/workbench", h.Workbench)
	r.GET("/products/:id/lifecycle-options", h.LifecycleOptions)
	r.GET("/products/:id/lifecycle/:kind", h.ListLifecycle)
	r.POST("/products/:id/lifecycle/:kind", h.SaveLifecycle)
	r.PUT("/products/:id/lifecycle/:kind/:recordID", h.SaveLifecycle)
	r.GET("/products/:id/distributions/:recordID/events", h.DistributionEvents)
	request := func(method, path, body, user string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User", user)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	for _, kind := range []string{"tasks", "versions", "distributions", "deployments"} {
		path := fmt.Sprintf("/products/%d/lifecycle/%s", p.ID, kind)
		if res := request("GET", path, "", ""); res.Code != 401 {
			t.Fatalf("authentication %s: %d", kind, res.Code)
		}
		if res := request("GET", path, "", "other"); res.Code != 404 {
			t.Fatalf("read isolation %s: %d", kind, res.Code)
		}
		if res := request("GET", path, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), "\"limit\":10") || !strings.Contains(res.Body.String(), "\"items\":[]") {
			t.Fatalf("empty paged list %s: %d %s", kind, res.Code, res.Body)
		}
	}
	for _, test := range []struct{ kind, body string }{
		{"tasks", "{}"}, {"tasks", "{\"title\":\"test\",\"kind\":\"action\",\"status\":\"todo\",\"priority\":\"normal\",\"due_date\":\"2026-02-30\"}"},
		{"versions", "{\"name\":\"1\",\"status\":\"planning\",\"build_url\":\"javascript:alert(1)\"}"},
		{"versions", "{\"name\":\"1\",\"status\":\"planning\",\"artifact_url\":\"https://user:password@example.com/a\"}"},
		{"distributions", "{\"version_id\":1,\"channel_id\":1,\"status\":\"nonsense\",\"platform_version\":\"1\"}"},
	} {
		if res := request("POST", fmt.Sprintf("/products/%d/lifecycle/%s", p.ID, test.kind), test.body, "owner"); res.Code != 400 {
			t.Fatalf("validation: %d %s", res.Code, res.Body)
		}
	}
	versionRes := request("POST", fmt.Sprintf("/products/%d/lifecycle/versions", p.ID), "{\"name\":\"1.0\",\"status\":\"testing\",\"owner_id\":2,\"product_id\":999}", "owner")
	var version struct{ Data model.ProductVersion }
	if err := json.Unmarshal(versionRes.Body.Bytes(), &version); err != nil || versionRes.Code != 200 || version.Data.ProductID != p.ID {
		t.Fatalf("version: %s %v", versionRes.Body, err)
	}
	body := fmt.Sprintf("{\"version_id\":%d,\"channel_id\":%d,\"platform_version\":\"1.0\",\"status\":\"in_review\"}", version.Data.ID, p.Channels[0].ID)
	res := request("POST", fmt.Sprintf("/products/%d/lifecycle/distributions", p.ID), body, "owner")
	var distribution struct{ Data model.ProductDistribution }
	if err := json.Unmarshal(res.Body.Bytes(), &distribution); err != nil || res.Code != 200 {
		t.Fatalf("distribution: %s %v", res.Body, err)
	}
	historyURL := fmt.Sprintf("/products/%d/distributions/%d/events", p.ID, distribution.Data.ID)
	if res := request("GET", historyURL, "", "other"); res.Code != 404 {
		t.Fatalf("history leak: %d", res.Code)
	}
	if res := request("GET", historyURL, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), "in_review") {
		t.Fatalf("history: %d %s", res.Code, res.Body)
	}
	updateURL := fmt.Sprintf("/products/%d/lifecycle/versions/%d", p.ID, version.Data.ID)
	update := fmt.Sprintf("{\"name\":\"1.0\",\"status\":\"completed\",\"revision\":%d}", version.Data.Revision)
	if res := request("PUT", updateURL, update, "other"); res.Code != 404 {
		t.Fatalf("foreign write: %d", res.Code)
	}
	if res := request("PUT", updateURL, update, "owner"); res.Code != 200 {
		t.Fatalf("update: %d %s", res.Code, res.Body)
	}
	if res := request("PUT", updateURL, update, "owner"); res.Code != 409 {
		t.Fatalf("lost update: %d %s", res.Code, res.Body)
	}
	if res := request("GET", fmt.Sprintf("/products/%d/lifecycle/tasks?version_id=bad", p.ID), "", "owner"); res.Code != 400 {
		t.Fatalf("filter validation: %d", res.Code)
	}
	if res := request("GET", "/products/workbench", "", "other"); res.Code != 200 || strings.Contains(res.Body.String(), "offline") {
		t.Fatalf("workbench leak: %d %s", res.Code, res.Body)
	}
}
