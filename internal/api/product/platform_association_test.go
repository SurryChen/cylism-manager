package productapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestProductPlatformAssociationAPI(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
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
	r.GET("/products/resource-options", h.Options)
	r.POST("/products", h.Create)
	r.GET("/products/:id", h.Get)
	r.PUT("/products/:id", h.Update)
	r.GET("/products/:id/lifecycle/:kind", h.ListLifecycle)
	r.POST("/products/:id/lifecycle/:kind", h.SaveLifecycle)
	request := func(method, path, body, user string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User", user)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	if res := request("GET", "/products/resource-options", "", ""); res.Code != 401 {
		t.Fatalf("unauthenticated options: %d %s", res.Code, res.Body)
	}
	if res := request("GET", "/products/resource-options", "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), "当前管理平台") {
		t.Fatalf("platform option: %d %s", res.Code, res.Body)
	}
	res := request("POST", "/products", `{"name":"Manager","platform_linked":true}`, "owner")
	var created struct{ Data model.Product }
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil || res.Code != 200 || !created.Data.PlatformLinked {
		t.Fatalf("create linked product: %d %s %v", res.Code, res.Body, err)
	}
	path := fmt.Sprintf("/products/%d", created.Data.ID)
	if res := request("GET", path, "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"platform_linked":true`) {
		t.Fatalf("read linked product: %d %s", res.Code, res.Body)
	}
	if res := request("PUT", path, `{"name":"stolen","platform_linked":false}`, "other"); res.Code != 404 {
		t.Fatalf("foreign product update: %d %s", res.Code, res.Body)
	}
	update := model.PlatformRelease{Source: "github", Image: "manager:1.0", Status: "succeeded", Detail: "private details"}
	if err := s.CreatePlatformRelease(&update); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"name":"1.0","status":"testing","platform_release_ids":[%d]}`, update.ID)
	if res := request("POST", path+"/lifecycle/versions", body, "owner"); res.Code != 200 {
		t.Fatalf("link platform release: %d %s", res.Code, res.Body)
	}
	if res := request("GET", path+"/lifecycle/deployments?version_id=1", "", "owner"); res.Code != 200 || !strings.Contains(res.Body.String(), `"source":"platform"`) || strings.Contains(res.Body.String(), "private details") {
		t.Fatalf("linked platform DTO: %d %s", res.Code, res.Body)
	}
	if res := request("GET", path+"/lifecycle/deployments", "", "other"); res.Code != 404 {
		t.Fatalf("foreign platform history: %d %s", res.Code, res.Body)
	}
	oversized := model.ProductVersion{Name: "over limit", Status: "testing", DeploymentIDs: make([]uint, 50), PlatformReleaseIDs: make([]uint, 51)}
	data, err := json.Marshal(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if res := request("POST", path+"/lifecycle/versions", string(data), "owner"); res.Code != 400 {
		t.Fatalf("combined deployment limit: %d %s", res.Code, res.Body)
	}
	if res := request("PUT", path, `{"name":"Manager","platform_linked":false}`, "owner"); res.Code != 200 {
		t.Fatalf("unlink product: %d %s", res.Code, res.Body)
	}
	if res := request("POST", path+"/lifecycle/versions", body, "owner"); res.Code != 409 {
		t.Fatalf("duplicate version: %d %s", res.Code, res.Body)
	}
	if res := request("POST", path+"/lifecycle/versions", strings.Replace(body, "1.0", "1.1", 1), "owner"); res.Code != 403 {
		t.Fatalf("unlinked platform release: %d %s", res.Code, res.Body)
	}
}
