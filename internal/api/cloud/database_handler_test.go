package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestDatabaseCredentialsAreHiddenAndBindingUsesTemplateEncryption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:database-handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte("a"), 32)
	h := NewDatabaseHandler(s, key, nil)
	r := gin.New()
	r.POST("/databases", h.Create)
	r.POST("/databases/:id/bindings", h.Bind)
	r.DELETE("/databases/:id/bindings/:bindingID", h.Unbind)
	r.DELETE("/databases/:id", h.Delete)
	request := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	w := request("/databases", `{"name":"external-db","mode":"external","host":"mysql.internal","port":3306,"database_name":"app_db","username":"app_user","password":"private-password"}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "private-password") {
		t.Fatalf("credential leaked or create failed: %s", w.Body.String())
	}
	var item model.DatabaseInstance
	if err := s.DB().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	project := model.Project{Name: "project"}
	s.DB().Create(&project)
	environment := model.Environment{ProjectID: project.ID, ClusterID: 2, Name: "test", Namespace: "project-test"}
	s.DB().Create(&environment)
	app := model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "app"}
	s.DB().Create(&app)
	template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: `{"image":"example/app","config":{"EXISTING":"kept"}}`, Revision: 1}
	s.DB().Create(&template)
	w = request("/databases/1/bindings", `{"application_id":1,"template_id":1,"prefix":"DATABASE"}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "private-password") {
		t.Fatalf("binding failed or credential leaked: %s", w.Body.String())
	}
	if err := s.DB().First(&template, template.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(template.Spec, "private-password") || !strings.Contains(template.Spec, "EXISTING") || !strings.Contains(template.Spec, "DATABASE_HOST") {
		t.Fatalf("template not safely updated: %s", template.Spec)
	}
	plain, err := security.Decrypt(key, template.EncryptedSecrets)
	if err != nil {
		t.Fatal(err)
	}
	var secrets map[string]string
	if err := json.Unmarshal([]byte(plain), &secrets); err != nil {
		t.Fatal(err)
	}
	if secrets["DATABASE_PASSWORD"] != "private-password" {
		t.Fatal("bound password missing from encrypted template")
	}
	managed := model.DatabaseInstance{Name: "other-cluster", Mode: "managed", ClusterID: 3, Host: "mysql.other.svc", Port: 3306, DatabaseName: "app_db", Username: "app_user", EncryptedPassword: item.EncryptedPassword, Status: "ready"}
	if err := s.DB().Create(&managed).Error; err != nil {
		t.Fatal(err)
	}
	w = request(fmt.Sprintf("/databases/%d/bindings", managed.ID), fmt.Sprintf(`{"application_id":%d,"template_id":%d}`, app.ID, template.ID))
	if w.Code == http.StatusOK {
		t.Fatal("cross-cluster managed database binding must be rejected")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/databases/%d", item.ID), nil))
	if w.Code == http.StatusOK {
		t.Fatal("bound database must not be deleted")
	}
	var binding model.DatabaseBinding
	if err := s.DB().Where("instance_id = ?", item.ID).First(&binding).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/databases/%d/bindings/%d", item.ID, binding.ID), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("unbind failed: %s", w.Body.String())
	}
	if err := s.DB().First(&template, template.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(template.Spec, "DATABASE_HOST") || strings.Contains(template.EncryptedSecrets, "private-password") {
		t.Fatal("database values remain after unbind")
	}
}
