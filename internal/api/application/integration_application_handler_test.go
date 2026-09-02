package applicationapi

import (
	"encoding/json"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"testing"
)

func TestIntegrationHandoffOnlyAllowsProtectedConsoleAndLoopbackDebugURL(t *testing.T) {
	r, s := setupApplicationRouter()
	app := createApplicationForReleaseRuntimeTest(t, s)
	protected := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "console.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessProtectedConsole}
	if err := s.CreateApplicationEndpoint(protected); err != nil {
		t.Fatalf("create protected endpoint: %v", err)
	}
	public := &model.ApplicationEndpoint{ApplicationID: app.ID, Exposure: application.ExposurePublic, Domain: "api.example.com", Path: "/", TLSEnabled: true, ServicePort: 80, AccessMode: model.ApplicationEndpointAccessPublic}
	if err := s.CreateApplicationEndpoint(public); err != nil {
		t.Fatalf("create public endpoint: %v", err)
	}

	local := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "http://localhost:5178/console?source=test"}))
	if local.Code != http.StatusOK || !strings.Contains(local.Body.String(), "http://localhost:5178/console?handoff_code=") || !strings.Contains(local.Body.String(), "source=test") {
		t.Fatalf("unexpected local handoff response: %d %s", local.Code, local.Body.String())
	}

	nonLoopback := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": protected.ID, "redirect_url": "https://example.com"}))
	if nonLoopback.Code != http.StatusBadRequest || !strings.Contains(nonLoopback.Body.String(), "本地联调地址无效") {
		t.Fatalf("unexpected non-loopback response: %d %s", nonLoopback.Code, nonLoopback.Body.String())
	}

	publicResult := serve(r, newJSONRequest(http.MethodPost, "/api/applications/1/integration-handoffs", gin.H{"endpoint_id": public.ID}))
	if publicResult.Code != http.StatusBadRequest || !strings.Contains(publicResult.Body.String(), "不是受保护控制台") {
		t.Fatalf("unexpected public endpoint response: %d %s", publicResult.Code, publicResult.Body.String())
	}
}

func TestIntegrationConfigMapReadsAndReplacesTemplateConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	app := createApplicationForReleaseRuntimeTest(t, s)
	if err := app.SetCapabilities([]string{"config-editor"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateApplication(app); err != nil {
		t.Fatal(err)
	}
	spec := application.ReleaseSpec{
		Image: "docker.io/example/app", ContainerPort: 8080, Replicas: 1,
		Resources:  application.ResourceSpec{RequestsCPU: "10m", RequestsMemory: "16Mi", LimitsCPU: "100m", LimitsMemory: "64Mi"},
		Config:     map[string]string{"config.yaml": "listen: :8443\n"},
		FileMounts: []application.FileMountSpec{{SourceType: application.FileMountSourceApplicationConfig, Key: "config.yaml", MountPath: "/etc/app/config.yaml", Managed: true}},
		Service:    application.ServiceSpec{Port: 8080, TargetPort: 8080, Protocol: application.ServiceProtocolTCP, Type: application.ServiceTypeClusterIP},
	}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	template := &model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "default", Enabled: true, Spec: string(snapshot)}
	if err := s.CreateApplicationDeploymentTemplate(template, true); err != nil {
		t.Fatal(err)
	}
	file := &model.ApplicationManagedFile{ApplicationID: app.ID, ResourceKind: application.FileMountSourceConfigMap, ResourceName: app.Name + "-config", Key: "config.yaml", MountPath: "/etc/app/config.yaml", CreatedBy: 1, Enabled: true}
	if err := s.CreateApplicationManagedFile(file); err != nil {
		t.Fatal(err)
	}

	h := NewApplicationHandler(s, []byte("01234567890123456789012345678901"))
	claims := &auth.DelegationClaims{UserID: 1, ProjectID: app.ProjectID, EnvironmentIDs: []uint{app.EnvironmentID}, ApplicationIDs: []uint{app.ID}, Capability: "config-editor", Actions: []string{"configmap:read", "configmap:write"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("delegation", claims); c.Set("user_id", uint(1)); c.Next() })
	r.GET("/api/integrations/applications/:id/configmaps/:configMapID", h.IntegrationGetManagedConfigMap)
	r.PUT("/api/integrations/applications/:id/configmaps/:configMapID", h.IntegrationReplaceManagedConfigMap)

	response := serve(r, newJSONRequest(http.MethodGet, "/api/integrations/applications/1/configmaps/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"content":"listen: :8443\n"`) || !strings.Contains(response.Body.String(), `"version":1`) {
		t.Fatalf("unexpected ConfigMap read response: %d %s", response.Code, response.Body.String())
	}
	response = serve(r, newJSONRequest(http.MethodPut, "/api/integrations/applications/1/configmaps/1", gin.H{"content": "listen: :9443\n", "expected_revision": 1}))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"version":2`) {
		t.Fatalf("unexpected ConfigMap replace response: %d %s", response.Code, response.Body.String())
	}
	updatedTemplate, err := s.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil || !strings.Contains(updatedTemplate.Spec, "listen: :9443") {
		t.Fatalf("template ConfigMap content was not updated: template=%#v err=%v", updatedTemplate, err)
	}
	response = serve(r, newJSONRequest(http.MethodPut, "/api/integrations/applications/1/configmaps/1", gin.H{"content": "stale", "expected_revision": 1}))
	if response.Code != http.StatusConflict {
		t.Fatalf("expected stale managed file update to fail, got %d %s", response.Code, response.Body.String())
	}
}
