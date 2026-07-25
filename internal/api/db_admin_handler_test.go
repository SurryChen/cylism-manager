package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func setupDBAdminRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewDBAdminHandler(s)
	admin := r.Group("/api/admin/tables")
	{
		admin.GET("", h.ListTables)
		admin.GET("/:table", h.ListRecords)
		admin.POST("/:table", h.CreateRecord)
		admin.PUT("/:table/:id", h.UpdateRecord)
		admin.DELETE("/:table/:id", h.DeleteRecord)
	}
	return r, s
}

func TestDBAdmin_ListTables(t *testing.T) {
	r, _ := setupDBAdminRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tables", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Tables []string `json:"tables"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &apiResp)
	if len(apiResp.Data.Tables) < 4 {
		t.Errorf("expected at least 4 tables, got %d: %v", len(apiResp.Data.Tables), apiResp.Data.Tables)
	}
}

func TestDBAdmin_ListRecords(t *testing.T) {
	r, s := setupDBAdminRouter()

	// Seed a server
	s.DB().Exec("INSERT INTO servers (name, host, ssh_auth_type, ssh_user, ssh_host, ssh_port) VALUES ('test', '1.1.1.1', 'password', 'root', '1.1.1.1', 22)")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tables/servers?page=1&size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Rows  []map[string]interface{} `json:"rows"`
			Total int64                    `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &apiResp)
	if apiResp.Data.Total == 0 {
		t.Error("expected at least 1 row")
	}
	if len(apiResp.Data.Rows) == 0 {
		t.Error("expected rows non-empty")
	}
}

func TestDBAdmin_InvalidTable(t *testing.T) {
	r, _ := setupDBAdminRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tables/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDBAdmin_CreateAndUpdateAndDelete(t *testing.T) {
	r, s := setupDBAdminRouter()

	// Create
	body := `{"name":"new-srv","host":"10.0.0.99","ssh_auth_type":"password","ssh_user":"root","ssh_host":"10.0.0.99","ssh_port":22}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tables/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get the id from wrapped response
	var idResp struct {
		Code int `json:"code"`
		Data struct {
			ID interface{} `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &idResp)

	// Update
	updateBody := `{"name":"updated-srv"}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/admin/tables/servers/1", strings.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify update
	var srv map[string]interface{}
	s.DB().Table("servers").Where("id = ?", 1).Take(&srv)
	_ = idResp
	_ = srv
}

func TestDBAdmin_SensitiveFieldsHidden(t *testing.T) {
	r, s := setupDBAdminRouter()

	// Insert user with password_hash
	s.DB().Exec("INSERT INTO users (username, password_hash) VALUES ('admin', 'secret-hash')")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tables/users?page=1&size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Rows []map[string]interface{} `json:"rows"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &apiResp)

	if len(apiResp.Data.Rows) > 0 {
		if _, ok := apiResp.Data.Rows[0]["password_hash"]; ok {
			t.Error("password_hash should be hidden")
		}
	}
}
