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

func TestPrivateGatewayAPIKeepsCredentialsEncryptedAndBindsServiceInItsCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New("file:gateway-handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.DB().DB()
	t.Cleanup(func() { _ = db.Close() })
	key := bytes.Repeat([]byte("g"), 32)
	password, _ := security.Encrypt(key, "private-db-password")
	instance := model.DatabaseInstance{Name: "rds", Mode: "external", Host: "rds.internal", Port: 3306, DatabaseName: "app_db", Username: "app_user", EncryptedPassword: password, Status: "registered"}
	server := model.Server{Name: "ecs", Host: "192.0.2.1"}
	cluster := model.Cluster{Name: "apps", Distribution: "k3s", Status: "ready"}
	for _, item := range []any{&instance, &server, &cluster} {
		if err := s.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	h := NewDatabaseHandler(s, key, nil).WithPrivateGateways(gateway.New(s, key, nil))
	r := gin.New()
	r.GET("/databases", h.List)
	r.GET("/databases/:id/gateway", h.GetPrivateGateway)
	r.PUT("/databases/:id/gateway", h.ConfigurePrivateGateway)
	r.POST("/databases/:id/gateway/actions", h.PrivateGatewayAction)
	r.GET("/databases/:id/gateway/tasks/:taskID/logs", h.PrivateGatewayTaskLogs)
	r.POST("/databases/:id/bindings", h.Bind)
	r.DELETE("/databases/:id", h.Delete)
	request := func(method, path, payload string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	path := fmt.Sprintf("/databases/%d", instance.ID)
	awaiting := instance
	awaiting.ConnectionMode = "gateway"
	if _, _, err := h.privateGatewayBinding(s.DB(), &awaiting, cluster.ID); err == nil {
		t.Fatal("gateway mode without a configured gateway must not bind directly")
	}
	w := request(http.MethodPut, path+"/gateway", fmt.Sprintf(`{"server_id":%d,"cluster_id":%d,"listen_ip":"100.80.1.1","listen_port":13306,"allowed_sources":["100.80.1.2/32"]}`, server.ID, cluster.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("configure failed: %s", w.Body.String())
	}
	var config model.DatabaseGateway
	s.DB().First(&config)
	var task model.DatabaseGatewayTask
	s.DB().First(&task)
	if task.Status != "queued" {
		t.Fatalf("API should enqueue: %+v", task)
	}
	s.DB().First(&instance, instance.ID)
	if instance.ConnectionMode != "gateway" {
		t.Fatal("configuring a gateway did not persist the connection mode")
	}
	w = request(http.MethodPost, path+"/gateway/actions", `{"action":"deploy"}`)
	if w.Code == http.StatusOK {
		t.Fatal("duplicate task accepted")
	}
	w = request(http.MethodGet, path+"/gateway", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), gateway.ServiceHost(config.ID)) || strings.Contains(w.Body.String(), "private-db-password") || strings.Contains(w.Body.String(), password) {
		t.Fatalf("unsafe/incomplete gateway view: %s", w.Body.String())
	}
	w = request(http.MethodGet, "/databases", "")
	if !strings.Contains(w.Body.String(), "private_gateway") || strings.Contains(w.Body.String(), "private-db-password") {
		t.Fatalf("gateway not shown safely in list: %s", w.Body.String())
	}
	w = request(http.MethodGet, path+"/gateway/tasks/999/logs", "")
	if w.Code != http.StatusNotFound {
		t.Fatal("unknown or foreign task exposed")
	}
	w = request(http.MethodDelete, path, "")
	if w.Code == http.StatusOK {
		t.Fatal("active gateway was orphaned by deletion")
	}
	project := model.Project{Name: "project"}
	s.DB().Create(&project)
	environment := model.Environment{Name: "app", ProjectID: project.ID, ClusterID: cluster.ID, Namespace: "apps"}
	s.DB().Create(&environment)
	app := model.Application{Name: "app", ProjectID: project.ID, EnvironmentID: environment.ID}
	s.DB().Create(&app)
	template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Spec: `{"image":"app:latest"}`, Revision: 1}
	s.DB().Create(&template)
	binding := fmt.Sprintf(`{"application_id":%d,"template_id":%d,"prefix":"MYSQL"}`, app.ID, template.ID)
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code == http.StatusOK {
		t.Fatal("unverified gateway bound to application")
	}
	s.DB().Model(&config).Update("status", "ready")
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code == http.StatusOK {
		t.Fatal("gateway with an active task bound to application")
	}
	s.DB().Model(&task).Update("status", "succeeded")
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code != http.StatusOK {
		t.Fatalf("bind failed: %s", w.Body.String())
	}
	s.DB().First(&template, template.ID)
	var spec struct {
		Config map[string]string `json:"config"`
	}
	json.Unmarshal([]byte(template.Spec), &spec)
	if spec.Config["MYSQL_HOST"] != gateway.ServiceHost(config.ID) || spec.Config["MYSQL_PORT"] != "3306" {
		t.Fatalf("binding did not use stable Service: %+v", spec.Config)
	}
	if strings.Contains(template.Spec, "private-db-password") {
		t.Fatal("binding leaked password into ordinary configuration")
	}
	plain, _ := security.Decrypt(key, template.EncryptedSecrets)
	if !strings.Contains(plain, "private-db-password") {
		t.Fatal("binding lost encrypted password")
	}
	s.DB().Model(&environment).Update("cluster_id", cluster.ID+1)
	w = request(http.MethodPost, path+"/bindings", binding)
	if w.Code == http.StatusOK {
		t.Fatal("cross-cluster Service binding accepted")
	}
}
