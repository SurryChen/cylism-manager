package productapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestProductDeleteAPIScopeAndClearConfirmation(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	current := model.Product{OwnerID: 1, Name: "current", Stage: "developing"}
	archived := model.Product{OwnerID: 1, Name: "archived", Stage: "archived"}
	other := model.Product{OwnerID: 2, Name: "other", Stage: "developing"}
	for _, p := range []*model.Product{&current, &archived, &other} {
		if err := s.SaveProduct(p, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(s)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") != "anonymous" {
			c.Set("user_id", uint(1))
		}
	})
	r.DELETE("/products/:id", h.Delete)
	r.POST("/products/clear", h.Clear)
	request := func(method, path, body, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	for _, tc := range []struct {
		method, path, body, user string
		status                   int
	}{
		{"DELETE", fmt.Sprintf("/products/%d", current.ID), "", "anonymous", 401},
		{"POST", "/products/clear", `{"confirmation":"删除全部产品"}`, "anonymous", 401},
		{"DELETE", "/products/0", "", "", 400},
		{"DELETE", "/products/nope", "", "", 400},
		{"DELETE", "/products/9999", "", "", 404},
		{"DELETE", fmt.Sprintf("/products/%d", other.ID), "", "", 404},
		{"POST", "/products/clear", `{}`, "", 400},
		{"POST", "/products/clear", `{"confirmation":"wrong"}`, "", 400},
	} {
		if res := request(tc.method, tc.path, tc.body, tc.user); res.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
	if products, err := s.ListProducts(1); err != nil || len(products) != 2 {
		t.Fatalf("rejected requests mutated data: %#v %v", products, err)
	}
	if res := request("DELETE", fmt.Sprintf("/products/%d", current.ID), "", ""); res.Code != 200 {
		t.Fatalf("delete: %d %s", res.Code, res.Body.String())
	}
	if _, err := s.GetProduct(1, archived.ID); err != nil {
		t.Fatal("single deletion affected other product")
	}
	res := request("POST", "/products/clear?scope=current&keyword=not-found", `{"confirmation":"删除全部产品"}`, "")
	var response struct {
		Data struct {
			Deleted int64 `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil || res.Code != 200 || response.Data.Deleted != 1 {
		t.Fatalf("clear: %d %s %v", res.Code, res.Body.String(), err)
	}
	if products, err := s.ListProducts(1); err != nil || len(products) != 0 {
		t.Fatalf("products remain: %#v %v", products, err)
	}
	if _, err := s.GetProduct(2, other.ID); err != nil {
		t.Fatal("other owner's product removed")
	}
}
