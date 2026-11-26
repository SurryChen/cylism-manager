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
	r.DELETE("/products/:id/lifecycle/task-groups/:recordID", h.DeleteTaskGroup)
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
	t.Run("distribution environment and deployment contracts", func(t *testing.T) {
		project := model.Project{OwnerID: 1, Name: "release project", Slug: "release-project"}
		if err := s.DB().Create(&project).Error; err != nil {
			t.Fatal(err)
		}
		environment := model.Environment{ProjectID: project.ID, Name: "testing", Kind: "testing", Namespace: "release-testing"}
		if err := s.DB().Create(&environment).Error; err != nil {
			t.Fatal(err)
		}
		application := model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "api", CreatedBy: 1}
		if err := s.CreateApplication(&application); err != nil {
			t.Fatal(err)
		}
		product := model.Product{OwnerID: 1, Name: "environment-linked", Channels: []model.ProductChannel{{Platform: "web", PublishStatus: "draft"}}}
		if err := s.SaveProduct(&product, []uint{project.ID}, nil); err != nil {
			t.Fatal(err)
		}
		release := model.Release{ApplicationID: application.ID, Sequence: 1, Image: "api:1.0", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}
		if err := s.DB().Create(&release).Error; err != nil {
			t.Fatal(err)
		}
		decode := func(res *httptest.ResponseRecorder, data any) {
			t.Helper()
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), data) != nil {
				t.Fatalf("request failed: %d %s", res.Code, res.Body)
			}
		}
		base := fmt.Sprintf("/products/%d/lifecycle/", product.ID)
		body := fmt.Sprintf(`{"channel_id":%d,"platform_version":"1.0","status":"preparing","environment_id":%d,"deployment_ids":[%d]}`, product.Channels[0].ID, environment.ID, release.ID)
		var saved struct{ Data model.ProductDistribution }
		decode(request("POST", base+"distributions", body, "owner"), &saved)
		if saved.Data.EnvironmentID != environment.ID || len(saved.Data.DeploymentIDs) != 1 || saved.Data.DeploymentIDs[0] != release.ID {
			t.Fatalf("missing saved associations: %+v", saved.Data)
		}
		var list struct {
			Data struct{ Items []model.ProductDistribution }
		}
		decode(request("GET", fmt.Sprintf("%sdistributions?environment_id=%d", base, environment.ID), "", "owner"), &list)
		if len(list.Data.Items) != 1 || list.Data.Items[0].EnvironmentName != environment.Name || len(list.Data.Items[0].DeploymentIDs) != 1 || list.Data.Items[0].DeploymentIDs[0] != release.ID {
			t.Fatalf("missing listed associations: %+v", list.Data.Items)
		}
		for _, kind := range []string{"distributions", "deployments"} {
			for _, value := range []string{"invalid", "-1", "0"} {
				res := request("GET", base+kind+"?environment_id="+value, "", "owner")
				if res.Code != 400 {
					t.Fatalf("invalid environment filter %s %q: %d %s", kind, value, res.Code, res.Body)
				}
			}
		}
		url := fmt.Sprintf("%sdistributions/%d", base, saved.Data.ID)
		legacy := fmt.Sprintf(`{"channel_id":%d,"platform_version":"1.0","status":"ready","revision":%d}`, product.Channels[0].ID, saved.Data.Revision)
		decode(request("PUT", url, legacy, "owner"), &saved)
		if saved.Data.EnvironmentID != environment.ID || len(saved.Data.DeploymentIDs) != 1 || saved.Data.DeploymentIDs[0] != release.ID {
			t.Fatalf("legacy client removed associations: %+v", saved.Data)
		}
		clear := fmt.Sprintf(`{"channel_id":%d,"platform_version":"1.0","status":"ready","revision":%d,"environment_id":0,"deployment_ids":[]}`, product.Channels[0].ID, saved.Data.Revision)
		decode(request("PUT", url, clear, "owner"), &saved)
		if saved.Data.EnvironmentID != 0 || len(saved.Data.DeploymentIDs) != 0 {
			t.Fatalf("explicit clear kept associations: %+v", saved.Data)
		}
		var history struct {
			Data struct {
				Items []model.ProductDistributionEvent
			}
		}
		decode(request("GET", fmt.Sprintf("/products/%d/distributions/%d/events", product.ID, saved.Data.ID), "", "owner"), &history)
		if len(history.Data.Items) != 3 || history.Data.Items[0].EnvironmentID != 0 || history.Data.Items[2].EnvironmentName != environment.Name || len(history.Data.Items[2].DeploymentIDs) != 1 || history.Data.Items[2].DeploymentIDs[0] != release.ID {
			t.Fatalf("history did not preserve environment snapshots: %+v", history.Data.Items)
		}
	})
	t.Run("groups preserve tasks and legacy updates", func(t *testing.T) {
		product := model.Product{OwnerID: 1, Name: "board"}
		if err := s.SaveProduct(&product, nil, nil); err != nil {
			t.Fatal(err)
		}
		base := fmt.Sprintf("/products/%d/lifecycle/", product.ID)
		decode := func(res *httptest.ResponseRecorder, data any) {
			t.Helper()
			if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), data) != nil {
				t.Fatalf("request failed: %d %s", res.Code, res.Body)
			}
		}
		var group struct{ Data model.ProductTaskGroup }
		decode(request("POST", base+"task-groups", `{"name":"  发布验证  "}`, "owner"), &group)
		if group.Data.ID == 0 {
			t.Fatal("group ID missing")
		}
		if group.Data.Name != "发布验证" {
			t.Fatalf("group name was not trimmed: %q", group.Data.Name)
		}
		for _, user := range []string{"", "other"} {
			expected := 404
			if user == "" {
				expected = 401
			}
			for _, method := range []string{"GET", "POST"} {
				if res := request(method, base+"task-groups", `{"name":"unauthorized"}`, user); res.Code != expected {
					t.Fatalf("group access %s %q: %d %s", method, user, res.Code, res.Body)
				}
			}
		}
		if res := request("POST", base+"task-groups", `{"name":"发布验证"}`, "owner"); res.Code != 409 {
			t.Fatalf("duplicate group: %d %s", res.Code, res.Body)
		}
		optionsURL := fmt.Sprintf("/products/%d/lifecycle-options", product.ID)
		if res := request("GET", optionsURL, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"task_groups":[{`) {
			t.Fatalf("group options: %d %s", res.Code, res.Body)
		}
		var task struct{ Data model.ProductTask }
		fields := `"title":"验证","kind":"action","status":"todo","priority":"normal"`
		body := fmt.Sprintf(`{%s,"group_id":%d}`, fields, group.Data.ID)
		decode(request("POST", base+"tasks", body, "owner"), &task)
		if task.Data.GroupID != group.Data.ID {
			t.Fatalf("assigned task: %+v", task.Data)
		}
		foreignBase := fmt.Sprintf("/products/%d/lifecycle/", p.ID)
		if res := request("POST", foreignBase+"tasks", body, "owner"); res.Code != 403 {
			t.Fatalf("cross-product assignment: %d %s", res.Code, res.Body)
		}
		taskURL := fmt.Sprintf("%stasks/%d", base, task.Data.ID)
		update := fmt.Sprintf(`{%s,"revision":%d}`, fields, task.Data.Revision)
		decode(request("PUT", taskURL, update, "owner"), &task)
		if task.Data.GroupID != group.Data.ID {
			t.Fatalf("legacy request cleared the group: %+v", task.Data)
		}
		readTasks := func(query string) []model.ProductTask {
			t.Helper()
			var list struct {
				Data struct{ Items []model.ProductTask }
			}
			decode(request("GET", base+"tasks?"+query, "", "owner"), &list)
			return list.Data.Items
		}
		groupFilter := fmt.Sprintf("group_id=%d", group.Data.ID)
		if open, done := readTasks(groupFilter+"&open=true"), readTasks(groupFilter+"&status=done"); len(open) != 1 || len(done) != 0 {
			t.Fatalf("unexpected board rows: open=%v, done=%v", open, done)
		}
		groupURL := fmt.Sprintf("%stask-groups/%d", base, group.Data.ID)
		decode(request("PUT", groupURL, `{"name":"上线验收","product_id":999}`, "owner"), &group)
		if group.Data.ProductID != product.ID || group.Data.Name != "上线验收" {
			t.Fatalf("group rename or ownership failed: %+v", group.Data)
		}
		if rows := readTasks(groupFilter); len(rows) != 1 || rows[0].GroupID != group.Data.ID {
			t.Fatalf("rename changed task grouping: %+v", rows)
		}
		var second struct{ Data model.ProductTaskGroup }
		decode(request("POST", base+"task-groups", `{"name":"研发"}`, "owner"), &second)
		var groupPage struct {
			Data struct {
				Items []model.ProductTaskGroup
				Total int64
			}
		}
		decode(request("GET", base+"task-groups?limit=1&offset=1", "", "owner"), &groupPage)
		if groupPage.Data.Total != 2 || len(groupPage.Data.Items) != 1 || groupPage.Data.Items[0].ID != second.Data.ID {
			t.Fatalf("group page: %+v", groupPage)
		}
		if res := request("PUT", groupURL, `{"name":"研发"}`, "owner"); res.Code != 409 {
			t.Fatalf("duplicate rename: %d %s", res.Code, res.Body)
		}
		if res := request("PUT", groupURL, `{"name":"unauthorized"}`, "other"); res.Code != 404 {
			t.Fatalf("foreign group rename: %d %s", res.Code, res.Body)
		}
		if res := request("GET", base+"tasks?group_id=-1", "", "owner"); res.Code != 400 {
			t.Fatalf("invalid group filter: %d %s", res.Code, res.Body)
		}
		if res := request("DELETE", groupURL, "", "other"); res.Code != 404 {
			t.Fatalf("foreign group delete: %d %s", res.Code, res.Body)
		}
		if res := request("DELETE", groupURL, "", "owner"); res.Code != 200 {
			t.Fatalf("group delete: %d %s", res.Code, res.Body)
		}
		if rows := readTasks("group_id=0"); len(rows) != 1 || rows[0].ID != task.Data.ID || rows[0].Revision != task.Data.Revision+1 {
			t.Fatalf("ungrouped tasks: %+v", rows)
		}
	})
	for _, kind := range []string{"tasks", "versions", "distributions", "deployments", "task-groups"} {
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
		{"task-groups", "{}"}, {"task-groups", `{"name":"   "}`}, {"task-groups", fmt.Sprintf(`{"name":%q}`, strings.Repeat("组", 65))},
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
