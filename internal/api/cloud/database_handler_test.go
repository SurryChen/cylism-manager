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
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestExternalDatabaseCanConfigureGatewayBeforeBusinessConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	key := bytes.Repeat([]byte("c"), 32)
	h := NewDatabaseHandler(s, nil, nil).WithPrivateGateways(gateway.New(s, key, nil))
	r := gin.New()
	r.POST("/databases", h.Create)
	r.GET("/databases", h.List)
	r.PUT("/databases/:id/connection", h.UpdateConnection)
	r.PUT("/databases/:id/gateway", h.ConfigurePrivateGateway)
	r.POST("/databases/:id/bindings", h.Bind)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	w := request(http.MethodPost, "/databases", `{"name":"rds","mode":"external","connection_mode":"gateway","host":"rds.internal","port":3306}`)
	if w.Code != http.StatusOK {
		t.Fatalf("endpoint registration requires credentials or encryption key: %s", w.Body.String())
	}
	var instance model.DatabaseInstance
	if err := s.DB().First(&instance).Error; err != nil {
		t.Fatal(err)
	}
	if instance.DatabaseName != "" || instance.Username != "" || instance.EncryptedPassword != "" {
		t.Fatal("registration invented business credentials")
	}
	server := model.Server{Name: "ecs", Host: "192.0.2.1"}
	cluster := model.Cluster{Name: "apps", Distribution: "k3s", Status: "ready"}
	project := model.Project{Name: "project"}
	for _, value := range []any{&server, &cluster, &project} {
		if err := s.DB().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	path := fmt.Sprintf("/databases/%d", instance.ID)
	w = request(http.MethodPut, path+"/gateway", fmt.Sprintf(`{"server_id":%d,"cluster_id":%d,"listen_ip":"100.80.1.1","listen_port":13306,"allowed_sources":["100.80.1.2/32"]}`, server.ID, cluster.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("gateway requires business credentials: %s", w.Body.String())
	}
	var config model.DatabaseGateway
	s.DB().First(&config)
	s.DB().Model(&config).Update("status", "ready")
	s.DB().Model(&model.DatabaseGatewayTask{}).Where("gateway_id = ?", config.ID).Update("status", "succeeded")
	environment := model.Environment{ProjectID: project.ID, ClusterID: cluster.ID, Name: "test", Namespace: "apps"}
	s.DB().Create(&environment)
	app := model.Application{ProjectID: project.ID, EnvironmentID: environment.ID, Name: "app"}
	s.DB().Create(&app)
	template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Spec: `{"image":"app:latest"}`, Revision: 1}
	s.DB().Create(&template)
	binding := fmt.Sprintf(`{"application_id":%d,"template_id":%d,"prefix":"MYSQL"}`, app.ID, template.ID)
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code == http.StatusOK || !strings.Contains(w.Body.String(), "请先配置业务数据库") {
		t.Fatalf("missing credentials not rejected clearly: %s", w.Body.String())
	}
	s.DB().First(&template, template.ID)
	if template.Revision != 1 || template.EncryptedSecrets != "" {
		t.Fatal("missing credentials changed the template")
	}
	h.key = key
	connection := `{"database_name":"app_db","username":"app_user","password":"business-password"}`
	w = request(http.MethodPut, path+"/connection", connection)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "business-password") || strings.Contains(w.Body.String(), "encrypted_password") {
		t.Fatalf("connection save failed or leaked credentials: %s", w.Body.String())
	}
	s.DB().First(&instance, instance.ID)
	plain, err := security.Decrypt(key, instance.EncryptedPassword)
	if err != nil || plain != "business-password" || instance.Host != "rds.internal" || instance.ConnectionMode != "gateway" {
		t.Fatal("business connection not encrypted or endpoint changed")
	}
	s.DB().First(&config, config.ID)
	if config.Status != "ready" || config.ListenIP != "100.80.1.1" {
		t.Fatal("saving credentials changed verified gateway")
	}
	w = request(http.MethodGet, "/databases", "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "business-password") || strings.Contains(w.Body.String(), instance.EncryptedPassword) {
		t.Fatal("list leaked credentials")
	}
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code != http.StatusOK {
		t.Fatalf("configured business connection cannot bind: %s", w.Body.String())
	}
	s.DB().First(&template, template.ID)
	if !strings.Contains(template.Spec, gateway.ServiceHost(config.ID)) || strings.Contains(template.Spec, "business-password") {
		t.Fatal("binding did not use Service or exposed password")
	}
	w = request(http.MethodPut, path+"/connection", `{"database_name":"other","username":"other","password":"changed"}`)
	if w.Code == http.StatusOK {
		t.Fatal("changed bound credentials without updating templates")
	}
	s.DB().First(&instance, instance.ID)
	if instance.DatabaseName != "app_db" || instance.Username != "app_user" {
		t.Fatal("rejected credential update changed the instance")
	}
}

func TestDatabaseRegistrationRejectsIncompleteBusinessCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	h := NewDatabaseHandler(s, bytes.Repeat([]byte("a"), 32), nil)
	r := gin.New()
	r.POST("/databases", h.Create)
	r.PUT("/databases/:id/connection", h.UpdateConnection)
	for _, body := range []string{
		`{"mode":"external","name":"partial","host":"rds.internal","port":3306,"database_name":"app_db"}`,
		`{"mode":"external","name":"partial","host":"rds.internal","port":3306,"username":"app_user","password":"secret"}`,
		`{"mode":"managed","name":"managed","cluster_id":1}`,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/databases", strings.NewReader(body)))
		if w.Code == http.StatusOK {
			t.Fatalf("incomplete credentials accepted: %s", body)
		}
	}
}

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
