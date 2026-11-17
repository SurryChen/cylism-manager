package shared

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResourceListPageFiltersBeforePaginationAndKeepsFacets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	items := []ResourceListKey{
		{Name: "api-c", Namespace: "prod", Node: "worker", Status: "Running", Restarts: 2},
		{Name: "api-a", Namespace: "prod", Node: "worker", Status: "Running", Restarts: 1},
		{Name: "api-b", Namespace: "prod", Status: "Pending"},
		{Name: "other", Namespace: "test", Node: "other", Status: "Failed"},
	}
	for _, query := range []string{
		"limit=1&offset=1&namespace=prod&node=worker&status=Running&keyword=API&restarts_only=true",
		"limit=1&offset=99&namespace=prod&node=worker&status=Running&keyword=API&restarts_only=true",
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		if InventoryNamespace(c) != "" {
			t.Fatal("paginated inventory must retain namespace facets")
		}
		ResourceListPage(c, items, func(item ResourceListKey) ResourceListKey { return item })
		var response struct {
			Data struct {
				Items                       []ResourceListKey `json:"items"`
				Total                       int               `json:"total"`
				Namespaces, Nodes, Statuses []string
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		data := response.Data
		if data.Total != 2 || len(data.Namespaces) != 2 || len(data.Nodes) != 2 || len(data.Statuses) != 3 {
			t.Fatalf("invalid filtered page/facets: %s", w.Body.String())
		}
		if c.Query("offset") == "1" && (len(data.Items) != 1 || data.Items[0].Name != "api-c") {
			t.Fatalf("unstable sort: %s", w.Body.String())
		}
		if c.Query("offset") == "99" && len(data.Items) != 0 {
			t.Fatalf("expected empty page: %s", w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?namespace=prod", nil)
	if InventoryNamespace(c) != "prod" {
		t.Fatal("legacy namespace scope changed")
	}
	ResourceListPage(c, items, func(item ResourceListKey) ResourceListKey { return item })
	var legacy struct{ Data []ResourceListKey }
	if err := json.Unmarshal(w.Body.Bytes(), &legacy); err != nil || len(legacy.Data) != 4 {
		t.Fatalf("legacy array response: %s %v", w.Body.String(), err)
	}
}
