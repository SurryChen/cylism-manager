package productapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestProjectWorkspaceAPIIsolationAndResourceProtection(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	p := model.Product{OwnerID: 1, Name: "No cluster required", Stage: "idea"}
	if err := s.SaveProduct(&p, nil, nil); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		switch c.GetHeader("X-User") {
		case "owner":
			c.Set("user_id", uint(1))
		case "other":
			c.Set("user_id", uint(2))
		}
	})
	r.GET("/projects/:projectID/workspace", h.Workspace)
	r.DELETE("/projects/:projectID/workspace", h.DeleteWorkspace)
	request := func(method, id, user string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/projects/"+id+"/workspace", nil)
		req.Header.Set("X-User", user)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	id := fmt.Sprint(*p.ProjectID)
	for _, method := range []string{"GET", "DELETE"} {
		for _, test := range []struct {
			id, user string
			status   int
		}{{id, "", 401}, {id, "other", 404}, {"bad", "owner", 400}, {"0", "owner", 400}, {"99999", "owner", 404}} {
			if res := request(method, test.id, test.user); res.Code != test.status {
				t.Fatalf("%s %s user %s: %d %s", method, test.id, test.user, res.Code, res.Body)
			}
		}
	}
	res := request("GET", id, "owner")
	var workspace struct{ Data model.Product }
	if err := json.Unmarshal(res.Body.Bytes(), &workspace); err != nil || res.Code != 200 || workspace.Data.ID != p.ID || workspace.Data.ProjectID == nil || *workspace.Data.ProjectID != *p.ProjectID {
		t.Fatalf("workspace identity: %s %v", res.Body, err)
	}
	env := model.Environment{ProjectID: *p.ProjectID, Name: "test", Namespace: "test"}
	if err := s.DB().Create(&env).Error; err != nil {
		t.Fatal(err)
	}
	if res := request("DELETE", id, "owner"); res.Code != 409 {
		t.Fatalf("resource protection: %d %s", res.Code, res.Body)
	}
	if res := request("GET", id, "owner"); res.Code != 200 {
		t.Fatalf("failed delete changed workspace: %d %s", res.Code, res.Body)
	}
	if err := s.DB().Delete(&env).Error; err != nil {
		t.Fatal(err)
	}
	if res := request("DELETE", id, "owner"); res.Code != 200 {
		t.Fatalf("delete: %d %s", res.Code, res.Body)
	}
	if res := request("GET", id, "owner"); res.Code != 404 {
		t.Fatalf("deleted workspace still exists: %d %s", res.Code, res.Body)
	}
}
