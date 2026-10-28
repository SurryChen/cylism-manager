package applicationapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestCatalogItemRejectsEnvironmentSpecificSettings(t *testing.T) {
	base := catalogItemRequest{
		Name: "订单系统", Slug: "orders", Enabled: true, EntryComponent: "order-api",
		Components: []catalogComponentRequest{{Name: "API", ApplicationName: "order-api", Spec: applicationservice.ReleaseSpec{
			Image: "registry.example.com/order-api", ContainerPort: 8080, Replicas: 1,
			Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
			Service:   applicationservice.ServiceSpec{Type: "ClusterIP", Ports: []applicationservice.ServicePortSpec{{Name: "http", Protocol: "TCP", Port: 8080, TargetPort: 8080}}},
		}}, {Name: "Web", ApplicationName: "order-web", Spec: applicationservice.ReleaseSpec{
			Image: "registry.example.com/order-web", ContainerPort: 8080, Replicas: 1,
			Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
			Service:   applicationservice.ServiceSpec{Type: "ClusterIP", Ports: []applicationservice.ServicePortSpec{{Name: "http", Protocol: "TCP", Port: 8080, TargetPort: 8080}}},
		}}},
	}
	if _, err := catalogItemFromRequest(base); err != nil {
		t.Fatalf("valid catalog item rejected: %v", err)
	}
	base.Components[0].DefaultVersion = "invalid tag"
	if _, err := catalogItemFromRequest(base); err == nil {
		t.Fatal("catalog must reject an invalid default version")
	}
	base.Components[0].DefaultVersion = ""
	if _, err := catalogItemFromRequest(catalogItemRequest{Name: base.Name, Slug: base.Slug, Enabled: true, EntryComponent: base.EntryComponent, Components: base.Components[:1]}); err != nil {
		t.Fatalf("catalog must accept one component: %v", err)
	}
	if _, err := catalogItemFromRequest(catalogItemRequest{Name: base.Name, Slug: base.Slug, Enabled: true}); err == nil {
		t.Fatal("enabled catalog must contain a component")
	}
	base.Components[0].Spec.Config = map[string]string{"API_URL": "http://order-web:8080"}
	if item, err := catalogItemFromRequest(base); err != nil {
		t.Fatalf("component config rejected: %v", err)
	} else if components, err := item.CatalogComponents(); err != nil || !strings.Contains(string(components[0].Spec), `"API_URL":"http://order-web:8080"`) {
		t.Fatalf("component config lost: %+v err=%v", components, err)
	}
	base.Components[0].Spec.Config = map[string]string{"invalid-key": "value"}
	if _, err := catalogItemFromRequest(base); err == nil {
		t.Fatal("catalog must reject invalid environment variable names")
	}
	base.Components[0].Spec.Config = nil
	base.Components[0].Spec.Secrets = map[string]string{"PASSWORD": "private"}
	if _, err := catalogItemFromRequest(base); err == nil {
		t.Fatal("catalog must reject embedded secrets")
	}
	base.Components[0].Spec.Secrets = nil
	base.Components = append(base.Components, base.Components[0])
	if _, err := catalogItemFromRequest(base); err == nil {
		t.Fatal("catalog must reject duplicate component names")
	}
}

func TestCatalogCreateAndImportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	project := &model.Project{Name: "测试项目", OwnerID: 1}
	if err := st.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	environment := &model.Environment{ProjectID: project.ID, ClusterID: 1, Name: "dev", Namespace: "catalog-dev"}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	h := newTestApplicationHandler(st, make([]byte, 32), nil)
	r := gin.New()
	r.POST("/api/application-catalog", h.CreateCatalogItem)
	r.POST("/api/application-catalog/from-environment", h.CreateCatalogFromEnvironment)
	r.GET("/api/application-catalog", h.ListCatalogItems)
	r.GET("/api/application-catalog/:catalogID", h.GetCatalogItem)
	r.PUT("/api/application-catalog/:catalogID", h.UpdateCatalogItem)
	r.GET("/api/application-catalog/installations", h.ListCatalogInstallations)
	r.POST("/api/application-catalog/import", h.ImportCatalogItems)
	spec := applicationservice.ReleaseSpec{
		Image: "registry.example.com/order-api", ContainerPort: 8080, Replicas: 1,
		Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
		Service:   applicationservice.ServiceSpec{Type: "ClusterIP", Ports: []applicationservice.ServicePortSpec{{Name: "http", Protocol: "TCP", Port: 8080, TargetPort: 8080}}},
	}
	item := catalogItemRequest{Name: "订单系统", Slug: "orders", EntryComponent: "order-web", Components: []catalogComponentRequest{
		{Name: "API", ApplicationName: "order-api", DefaultVersion: "1.0.0", Spec: spec},
		{Name: "Web", ApplicationName: "order-web", Spec: spec},
	}}
	created := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog", catalogItemRequest{Name: item.Name, Slug: item.Slug}))
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	id := responseID(t, created.Body.Bytes())
	payload := gin.H{"project_id": project.ID, "environment_id": environment.ID, "item_ids": []uint{id}}
	if draftImport := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/import", payload)); draftImport.Code != http.StatusBadRequest {
		t.Fatalf("draft import should fail: %d %s", draftImport.Code, draftImport.Body.String())
	}
	detail := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/application-catalog/%d", id), nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"enabled":false`) || !strings.Contains(detail.Body.String(), `"components":[]`) {
		t.Fatalf("draft detail: %d %s", detail.Code, detail.Body.String())
	}
	item.Revision = 1
	item.Enabled = true
	updated := serve(r, newJSONRequest(http.MethodPut, fmt.Sprintf("/api/application-catalog/%d", id), item))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"enabled":true`) {
		t.Fatalf("enable: %d %s", updated.Code, updated.Body.String())
	}
	item.Revision = 2
	item.Enabled = false
	disabled := serve(r, newJSONRequest(http.MethodPut, fmt.Sprintf("/api/application-catalog/%d", id), item))
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", disabled.Code, disabled.Body.String())
	}
	if disabledImport := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/import", payload)); disabledImport.Code != http.StatusBadRequest {
		t.Fatalf("disabled import should fail: %d %s", disabledImport.Code, disabledImport.Body.String())
	}
	item.Revision = 3
	item.Enabled = true
	if enabled := serve(r, newJSONRequest(http.MethodPut, fmt.Sprintf("/api/application-catalog/%d", id), item)); enabled.Code != http.StatusOK {
		t.Fatalf("re-enable: %d %s", enabled.Code, enabled.Body.String())
	}
	listed := serve(r, newJSONRequest(http.MethodGet, "/api/application-catalog", nil))
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"entry_component":"order-web"`) || !strings.Contains(listed.Body.String(), `"application_name":"order-api"`) {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	imported := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/import", payload))
	if imported.Code != http.StatusOK {
		t.Fatalf("import: %d %s", imported.Code, imported.Body.String())
	}
	installations := serve(r, newJSONRequest(http.MethodGet, fmt.Sprintf("/api/application-catalog/installations?project_id=%d&environment_id=%d", project.ID, environment.ID), nil))
	if installations.Code != http.StatusOK || !strings.Contains(installations.Body.String(), `"entry_application_id":`) || !strings.Contains(installations.Body.String(), `"is_entry":true`) {
		t.Fatalf("list installations: %d %s", installations.Code, installations.Body.String())
	}
	apps, err := st.ListApplications(project.ID, environment.ID)
	if err != nil || len(apps) != 2 {
		t.Fatalf("imported apps: %+v err=%v", apps, err)
	}
	for _, app := range apps {
		if app.DefaultDeploymentTemplateID == nil {
			t.Fatalf("missing default template: %+v", app)
		}
		template, err := st.GetDefaultApplicationDeploymentTemplate(app.ID)
		if err != nil {
			t.Fatalf("default template: %v", err)
		}
		if app.Name == "order-api" && template.DefaultVersion != "1.0.0" {
			t.Fatalf("imported template lost default version: %+v", template)
		}
	}
	var webAppID uint
	for _, app := range apps {
		if app.Name == "order-web" {
			webAppID = app.ID
		}
	}
	if err := st.CreateRelease(&model.Release{ApplicationID: webAppID, Sequence: 1, Image: "registry.example.com/order-web:3.0.0", Version: "3.0.0", DesiredSpec: "{}", Status: model.ReleaseStatusSucceeded, CreatedBy: 1}); err != nil {
		t.Fatal(err)
	}
	fromEnvironment := gin.H{"project_id": project.ID, "environment_id": environment.ID, "application_ids": []uint{apps[0].ID, apps[1].ID}, "entry_application_id": apps[1].ID, "name": "现有订单系统", "slug": "existing-orders"}
	createdFromEnvironment := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/from-environment", fromEnvironment))
	if createdFromEnvironment.Code != http.StatusOK || !strings.Contains(createdFromEnvironment.Body.String(), `"enabled":false`) || !strings.Contains(createdFromEnvironment.Body.String(), `"entry_component":"`+apps[1].Name+`"`) || !strings.Contains(createdFromEnvironment.Body.String(), `"default_version":"1.0.0"`) || !strings.Contains(createdFromEnvironment.Body.String(), `"default_version":"3.0.0"`) {
		t.Fatalf("create from environment: %d %s", createdFromEnvironment.Code, createdFromEnvironment.Body.String())
	}
	fromEnvironment["application_ids"] = []uint{apps[0].ID}
	if one := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/from-environment", fromEnvironment)); one.Code != http.StatusBadRequest {
		t.Fatalf("single component package should fail: %d %s", one.Code, one.Body.String())
	}
	conflict := serve(r, newJSONRequest(http.MethodPost, "/api/application-catalog/import", payload))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("duplicate import: %d %s", conflict.Code, conflict.Body.String())
	}
}

func TestCatalogComponentFromTemplateRejectsNonPortableSettings(t *testing.T) {
	spec := applicationservice.ReleaseSpec{
		Image: "registry.example.com/order-api", Version: "v1", RegistryID: 8, ContainerPort: 8080, Replicas: 1,
		Resources: applicationservice.ResourceSpec{RequestsCPU: "100m", RequestsMemory: "128Mi", LimitsCPU: "500m", LimitsMemory: "512Mi"},
		Service:   applicationservice.ServiceSpec{Type: "NodePort", Ports: []applicationservice.ServicePortSpec{{Name: "http", Protocol: "TCP", Port: 8080, TargetPort: 8080, NodePort: 30080}}},
		Endpoint:  applicationservice.EndpointSpec{Exposure: "public", Domain: "example.com"},
	}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	app := model.Application{Name: "order-api", WorkloadKind: "deployment"}
	template := model.ApplicationDeploymentTemplate{Spec: string(snapshot)}
	component, err := catalogComponentFromTemplate(app, template)
	if err != nil || component.Spec.Version != "" || component.Spec.RegistryID != 0 || component.Spec.Service.Type != "ClusterIP" || component.Spec.Service.Ports[0].NodePort != 0 || component.Spec.Endpoint.Exposure != "cluster" {
		t.Fatalf("portable component: %+v err=%v", component, err)
	}
	for _, field := range []string{"config", "secret"} {
		fileSpec := spec
		if field == "config" {
			fileSpec.ConfigFileKeys = []string{"env"}
		} else {
			fileSpec.SecretFileKeys = []string{"env"}
		}
		encoded, err := json.Marshal(fileSpec)
		if err != nil {
			t.Fatal(err)
		}
		template.Spec = string(encoded)
		if _, err := catalogComponentFromTemplate(app, template); err == nil {
			t.Fatal("file-only settings must not be silently dropped by catalog import")
		}
	}
	ndots := 1
	spec.DNSNDots = &ndots
	snapshot, err = json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	template.Spec = string(snapshot)
	if _, err := catalogComponentFromTemplate(app, template); err == nil {
		t.Fatal("custom DNS must not be silently dropped by the catalog editor")
	}
	spec.DNSNDots = nil
	snapshot, err = json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	template.Spec = string(snapshot)
	template.EncryptedSecrets = "encrypted"
	if _, err := catalogComponentFromTemplate(app, template); err == nil {
		t.Fatal("template secrets must not be imported into catalog")
	}
}
