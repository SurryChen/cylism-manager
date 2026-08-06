package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/crypto"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestAssistantProviderRequiresOpenAIResponsesAPI(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	handler := NewAssistantHandler(nil, key)

	provider, err := handler.providerFromRequest(assistantProviderRequest{
		Name:         "OpenAI",
		ProviderType: "openai_responses",
		BaseURL:      "https://responses.example.test/v1/",
		Model:        "gpt-4o-mini",
		APIKey:       "sk-test",
		Enabled:      true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ProviderType != "openai_responses" {
		t.Fatalf("provider type = %q, want openai_responses", provider.ProviderType)
	}
	if provider.BaseURL != "https://responses.example.test/v1" {
		t.Fatalf("base URL = %q, want normalized Responses API endpoint", provider.BaseURL)
	}
	apiKey, err := crypto.Decrypt(key, provider.APIKeyEncrypted)
	if err != nil || apiKey != "sk-test" {
		t.Fatalf("stored API key was not encrypted correctly: %q, %v", apiKey, err)
	}

	for _, providerType := range []string{"", "openai", "openai_compatible"} {
		_, err := handler.providerFromRequest(assistantProviderRequest{
			Name:         "Legacy",
			ProviderType: providerType,
			Model:        "gpt-4o-mini",
			APIKey:       "sk-test",
			Enabled:      true,
		}, nil)
		if err == nil || !strings.Contains(err.Error(), "OpenAI Responses API") {
			t.Fatalf("provider type %q error = %v, want Responses API validation error", providerType, err)
		}
	}
}

func TestAssistantInstallRejectsPersistedLegacyProvider(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	legacy := &model.AssistantProvider{Name: "Legacy", ProviderType: "openai_compatible", Model: "gpt-4o-mini", Enabled: true}
	if err := st.CreateAssistantProvider(legacy); err != nil {
		t.Fatal(err)
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = originalK8s }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assistant/install", NewAssistantHandler(st, []byte("01234567890123456789012345678901")).Install)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/assistant/install", gin.H{"provider_id": legacy.ID}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "请选择已启用的模型提供商") {
		t.Fatalf("legacy provider install response: %d %s", response.Code, response.Body.String())
	}
}

func TestAssistantStatusSeparatesModelConfigurationFromRuntimeReadiness(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	provider := &model.AssistantProvider{Name: "Responses", ProviderType: assistantProviderTypeResponses, Model: "gpt-4o-mini", APIKeyEncrypted: "encrypted", Enabled: true}
	if err := st.CreateAssistantProvider(provider); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10)); err != nil {
		t.Fatal(err)
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "default"},
		Status:     appsv1.DeploymentStatus{UpdatedReplicas: 1, AvailableReplicas: 1},
	})}
	defer func() { K8s = originalK8s }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/assistant/status", NewAssistantHandler(st, []byte("01234567890123456789012345678901")).Status)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/assistant/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("assistant status response: %d %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"configured":true`, `"state":"ready"`, `"ready_replicas":1`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("assistant status missing %s: %s", expected, response.Body.String())
		}
	}
}

func TestAssistantUpdateDefaultProviderReconcilesRuntime(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	encryptedKey, err := crypto.Encrypt(key, "old-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := &model.AssistantProvider{Name: "Responses", ProviderType: assistantProviderTypeResponses, Model: "gpt-4o-mini", APIKeyEncrypted: encryptedKey, Enabled: true}
	if err := st.CreateAssistantProvider(provider); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10)); err != nil {
		t.Fatal(err)
	}

	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent-audit", Namespace: "default"}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{"kubernetes.io/hostname": "worker-a"},
				Containers:   []corev1.Container{{Name: "agent", Image: "example.test/agent:old"}},
			}}},
		},
	)}
	defer func() { K8s = originalK8s }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/api/assistant/providers/:id", NewAssistantHandler(st, key).UpdateProvider)
	response := serve(router, newJSONRequest(http.MethodPut, "/api/assistant/providers/1", gin.H{
		"name":          "Responses",
		"provider_type": assistantProviderTypeResponses,
		"base_url":      "https://responses.example.test/v1",
		"model":         "gpt-4.1-mini",
		"api_key":       "new-key",
		"enabled":       true,
	}))
	if response.Code != http.StatusOK {
		t.Fatalf("provider update response: %d %s", response.Code, response.Body.String())
	}

	configMap, err := K8s.Clientset.CoreV1().ConfigMaps("default").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if configMap.Data["OPS_AGENT_MODEL"] != "gpt-4.1-mini" || configMap.Data["OPENAI_BASE_URL"] != "https://responses.example.test/v1" {
		t.Fatalf("unexpected reconciled Runtime config: %#v", configMap.Data)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("default").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Annotations["cylism.dev/runtime-config-version"] == "" {
		t.Fatal("Runtime update must change the Pod template to trigger a rollout")
	}
}

func TestAssistantDeleteRejectsDefaultProvider(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	provider := &model.AssistantProvider{Name: "Responses", ProviderType: assistantProviderTypeResponses, Model: "gpt-4o-mini", Enabled: true}
	if err := st.CreateAssistantProvider(provider); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSystemConfig(assistantDefaultProviderConfigKey, strconv.FormatUint(uint64(provider.ID), 10)); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/assistant/providers/:id", NewAssistantHandler(st, []byte("01234567890123456789012345678901")).DeleteProvider)
	response := serve(router, newJSONRequest(http.MethodDelete, "/api/assistant/providers/1", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "默认模型提供商") {
		t.Fatalf("default provider delete response: %d %s", response.Code, response.Body.String())
	}
}
