package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes/fake"
)

func setupRuntimeRouter() (*gin.Engine, *store.Store) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	r := gin.New()
	h := NewRuntimeHandler(s, []byte("01234567890123456789012345678901"), nil)
	r.GET("/api/runtimes/catalog", h.Catalog)
	r.GET("/api/runtimes", h.List)
	r.POST("/api/runtimes", h.Create)
	r.PUT("/api/runtimes/:id", h.Update)
	return r, s
}

func TestRuntimeHandlerCreateAndUpdateKeepsSecretHidden(t *testing.T) {
	r, s := setupRuntimeRouter()
	create := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{
		"name": "nanobot-main", "runtime_type": "nanobot", "image": "ghcr.io/example/nanobot:latest", "api_key": "secret-key",
	}))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "secret-key") || !strings.Contains(create.Body.String(), "api_key_configured") {
		t.Fatalf("API key must be hidden and configured state exposed: %s", create.Body.String())
	}
	id := responseID(t, create.Body.Bytes())
	instance, err := s.GetRuntime(id)
	if err != nil || instance.EncryptedAPIKey == "" || instance.EncryptedRuntimeAPIKey != "" || instance.Namespace != "cylism-assistant" || instance.APIStyle != "responses" {
		t.Fatalf("unexpected runtime persistence: %+v err=%v", instance, err)
	}
	update := serve(r, newJSONRequest(http.MethodPut, "/api/runtimes/"+itoa(id), gin.H{"name": "nanobot-main", "image": instance.Image, "model_name": "qwen-max"}))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	instance, _ = s.GetRuntime(id)
	if instance.ModelName != "qwen-max" || instance.EncryptedAPIKey == "" {
		t.Fatalf("update must retain API key: %+v", instance)
	}
}

func TestRuntimeHandlerRejectsCustomNanobotConfig(t *testing.T) {
	r, _ := setupRuntimeRouter()
	response := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "nanobot-main", "runtime_type": "nanobot", "image": "example/nanobot:latest", "config": gin.H{"tools": gin.H{"exec": gin.H{"enable": true}}}}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "不支持覆盖") {
		t.Fatalf("expected protected config rejection, got %d %s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerUsesAdapterEndpointForManagedRuntime(t *testing.T) {
	r, s := setupRuntimeRouter()
	response := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "nanobot-main", "runtime_type": "nanobot", "image": "example/nanobot:latest", "port": 8080, "health_path": "/custom-health"}))
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	instance, err := s.GetRuntime(responseID(t, response.Body.Bytes()))
	if err != nil || instance.Port != runtime.NanobotAPIPort || instance.HealthPath != "/health" {
		t.Fatalf("managed endpoint must be adapter-owned: %+v err=%v", instance, err)
	}
}

func TestRuntimeHandlerGeneratesAndHidesRuntimeAPICredentialOnDeploy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	manager := runtime.NewKubernetesManager(&k8s.Client{Clientset: fake.NewSimpleClientset()})
	r := gin.New()
	h := NewRuntimeHandler(s, []byte("01234567890123456789012345678901"), manager)
	r.POST("/api/runtimes", h.Create)
	r.POST("/api/runtimes/:id/deploy", h.Deploy)
	r.GET("/api/runtimes/:id", h.Get)

	create := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "nanobot-main", "runtime_type": "nanobot", "image": "example/nanobot:latest", "api_key": "model-key", "model_name": "gpt-test", "model_base_url": "https://provider.example/v1"}))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	id := responseID(t, create.Body.Bytes())
	deploy := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/deploy", nil))
	if deploy.Code != http.StatusOK {
		t.Fatalf("deploy status = %d: %s", deploy.Code, deploy.Body.String())
	}
	stored, err := s.GetRuntime(id)
	if err != nil || stored.EncryptedRuntimeAPIKey == "" {
		t.Fatalf("runtime credential was not generated: %+v err=%v", stored, err)
	}
	if strings.Contains(deploy.Body.String(), "model-key") || strings.Contains(deploy.Body.String(), stored.EncryptedRuntimeAPIKey) {
		t.Fatalf("credentials leaked from deploy response: %s", deploy.Body.String())
	}
}

func TestRuntimeHandlerRejectsManagedDeploymentWithoutModelCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _ := store.New(":memory:")
	manager := runtime.NewKubernetesManager(&k8s.Client{Clientset: fake.NewSimpleClientset()})
	r := gin.New()
	h := NewRuntimeHandler(s, []byte("01234567890123456789012345678901"), manager)
	r.POST("/api/runtimes", h.Create)
	r.POST("/api/runtimes/:id/deploy", h.Deploy)

	create := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "nanobot-main", "runtime_type": "nanobot", "image": "example/nanobot:latest", "model_name": "gpt-test", "model_base_url": "https://provider.example/v1"}))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	deploy := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(responseID(t, create.Body.Bytes()))+"/deploy", nil))
	if deploy.Code != http.StatusBadRequest || !strings.Contains(deploy.Body.String(), "模型 API 密钥") {
		t.Fatalf("expected model credential validation, got %d %s", deploy.Code, deploy.Body.String())
	}
}

func TestRuntimeHandlerRejectsUnsupportedType(t *testing.T) {
	r, _ := setupRuntimeRouter()
	response := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "pi", "runtime_type": "pi", "image": "example/pi:latest"}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "不支持的 Runtime 类型") {
		t.Fatalf("expected unsupported type validation, got %d %s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerCatalog(t *testing.T) {
	r, _ := setupRuntimeRouter()
	response := serve(r, newJSONRequest(http.MethodGet, "/api/runtimes/catalog", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), model.RuntimeTypeNanobot) || !strings.Contains(response.Body.String(), "supported_model_protocols") {
		t.Fatalf("unexpected catalog response: %d %s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerAllowsExternalConnectionWithoutKubernetesImage(t *testing.T) {
	r, s := setupRuntimeRouter()
	create := serve(r, newJSONRequest(http.MethodPost, "/api/runtimes", gin.H{"name": "nanobot-remote", "runtime_type": "nanobot", "deployment_mode": "external", "endpoint_url": "http://127.0.0.1:9"}))
	if create.Code != http.StatusOK {
		t.Fatalf("external runtime create status = %d: %s", create.Code, create.Body.String())
	}
	instance, err := s.GetRuntime(responseID(t, create.Body.Bytes()))
	if err != nil || instance.DeploymentMode != model.RuntimeDeploymentExternal || instance.Image != "external" {
		t.Fatalf("unexpected external runtime: %+v err=%v", instance, err)
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
