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
	r.DELETE("/products/:id/lifecycle/tasks/:recordID", h.DeleteTask)
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
	body := fmt.Sprintf("{\"version_id\":%d,\"channel_id\":%d,\"status\":\"in_review\"}", version.Data.ID, p.Channels[0].ID)
	res := request("POST", fmt.Sprintf("/products/%d/lifecycle/distributions", p.ID), body, "owner")
	var distribution struct{ Data model.ProductDistribution }
	if err := json.Unmarshal(res.Body.Bytes(), &distribution); err != nil || res.Code != 200 || distribution.Data.PlatformVersion != "1.0" {
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

	t.Run("task selection and deletion", func(t *testing.T) {
		tasks := fmt.Sprintf("/products/%d/lifecycle/tasks", p.ID)
		versions := fmt.Sprintf("/products/%d/lifecycle/versions", p.ID)
		res := request("POST", tasks, `{"title":"验收","kind":"action","status":"done","priority":"normal"}`, "owner")
		var task struct{ Data model.ProductTask }
		if err := json.Unmarshal(res.Body.Bytes(), &task); err != nil || res.Code != 200 || task.Data.ID == 0 {
			t.Fatalf("create task: %d %s %v", res.Code, res.Body, err)
		}
		taskURL := fmt.Sprintf("%s/%d", tasks, task.Data.ID)
		versionBody := fmt.Sprintf(`{"name":"2.0","status":"releasing","task_ids":[%d]}`, task.Data.ID)
		res = request("POST", versions, versionBody, "owner")
		var release struct{ Data model.ProductVersion }
		if err := json.Unmarshal(res.Body.Bytes(), &release); err != nil || res.Code != 200 || len(release.Data.TaskIDs) != 1 {
			t.Fatalf("assign task: %d %s %v", res.Code, res.Body, err)
		}
		if res := request("GET", tasks+"?assignable=true", "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"total":1`) {
			t.Fatalf("shared task missing from the release picker: %d %s", res.Code, res.Body)
		}
		candidates := fmt.Sprintf("%s?assignable=true&version_id=%d", tasks, release.Data.ID)
		if res := request("GET", candidates, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"total":1`) {
			t.Fatalf("assigned task missing from its version: %d %s", res.Code, res.Body)
		}
		var unit struct{ Data model.ProductReleaseUnit }
		res = request("POST", fmt.Sprintf("/products/%d/lifecycle/release-units", p.ID), `{"name":"Android"}`, "owner")
		if err := json.Unmarshal(res.Body.Bytes(), &unit); err != nil || res.Code != 200 || unit.Data.ID == 0 {
			t.Fatalf("create release unit: %d %s %v", res.Code, res.Body, err)
		}
		otherVersionBody := fmt.Sprintf(`{"name":"2.0","released_on":"2026-11-24","release_unit_id":%d,"task_ids":[%d]}`, unit.Data.ID, task.Data.ID)
		var secondRelease struct{ Data model.ProductVersion }
		res = request("POST", versions, otherVersionBody, "owner")
		if err := json.Unmarshal(res.Body.Bytes(), &secondRelease); err != nil || res.Code != 200 || secondRelease.Data.Status != "completed" {
			t.Fatalf("second release: %d %s %v", res.Code, res.Body, err)
		}
		for _, id := range []uint{release.Data.ID, secondRelease.Data.ID} {
			path := fmt.Sprintf("%s?version_id=%d", tasks, id)
			if res := request("GET", path, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"total":1`) {
				t.Fatalf("task missing from release %d: %d %s", id, res.Code, res.Body)
			}
		}
		for user, status := range map[string]int{"": 401, "other": 404} {
			if res := request("DELETE", taskURL, "", user); res.Code != status {
				t.Fatalf("delete authorization for %q: %d %s", user, res.Code, res.Body)
			}
		}
		if res := request("DELETE", taskURL, "", "owner"); res.Code != 200 {
			t.Fatalf("delete: %d %s", res.Code, res.Body)
		}
		if res := request("GET", candidates, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"total":0`) {
			t.Fatalf("deleted task still visible: %d %s", res.Code, res.Body)
		}
		releaseURL := fmt.Sprintf("%s/%d", versions, release.Data.ID)
		staleBody := fmt.Sprintf(`{"name":"2.0","status":"releasing","revision":%d}`, release.Data.Revision)
		if res := request("PUT", releaseURL, staleBody, "owner"); res.Code != 409 {
			t.Fatalf("deletion did not invalidate the stale version: %d %s", res.Code, res.Body)
		}
		if res := request("GET", versions, "", "owner"); res.Code != 200 || strings.Contains(res.Body.String(), fmt.Sprintf(`"task_ids":[%d]`, task.Data.ID)) {
			t.Fatalf("version still reports a deleted task: %d %s", res.Code, res.Body)
		}
	})
}
