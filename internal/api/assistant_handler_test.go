package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

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
		ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "cylism-assistant"},
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
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent-audit", Namespace: "cylism-assistant"}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "cylism-assistant"},
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

	configMap, err := K8s.Clientset.CoreV1().ConfigMaps("cylism-assistant").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if configMap.Data["OPS_AGENT_MODEL"] != "gpt-4.1-mini" || configMap.Data["OPENAI_BASE_URL"] != "https://responses.example.test/v1" {
		t.Fatalf("unexpected reconciled Runtime config: %#v", configMap.Data)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("cylism-assistant").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Annotations["cylism.dev/runtime-config-version"] == "" {
		t.Fatal("Runtime update must change the Pod template to trigger a rollout")
	}
}

func TestAssistantDefaultRuntimeURLUsesDedicatedNamespace(t *testing.T) {
	t.Setenv("CYLISM_ASSISTANT_RUNTIME_URL", "")
	handler := NewAssistantHandler(nil, nil)
	if handler.runtimeURL != "http://cylism-ops-agent.cylism-assistant.svc:8080" {
		t.Fatalf("runtime URL = %q", handler.runtimeURL)
	}
}

func TestAssistantRuntimeUsesLegacyServiceWhileMigrationIsActive(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAssistantRuntimeMigration(&model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "default", SourcePVCName: "cylism-ops-agent-audit", TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit", TargetNodeName: "worker-a", Storage: "1Gi", Status: model.AssistantRuntimeMigrationCopying}); err != nil {
		t.Fatal(err)
	}
	handler := NewAssistantHandler(st, nil)
	if got := handler.runtimeURLForRequest(); got != "http://cylism-ops-agent.default.svc:8080" {
		t.Fatalf("migration runtime URL = %q", got)
	}
}

func TestAssistantRuntimeCustomURLOverridesMigrationRouting(t *testing.T) {
	t.Setenv("CYLISM_ASSISTANT_RUNTIME_URL", "https://runtime.example.test")
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAssistantRuntimeMigration(&model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "default", SourcePVCName: "cylism-ops-agent-audit", TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit", TargetNodeName: "worker-a", Storage: "1Gi", Status: model.AssistantRuntimeMigrationCopying}); err != nil {
		t.Fatal(err)
	}
	if got := NewAssistantHandler(st, nil).runtimeURLForRequest(); got != "https://runtime.example.test" {
		t.Fatalf("custom runtime URL = %q", got)
	}
}

func TestAssistantStatusIncludesActiveRuntimeMigration(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAssistantRuntimeMigration(&model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "default", SourcePVCName: "cylism-ops-agent-audit", TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit", TargetNodeName: "worker-a", Storage: "1Gi", Status: model.AssistantRuntimeMigrationCopying}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/assistant/status", NewAssistantHandler(st, nil).Status)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/assistant/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"migration":{"id":1`) || !strings.Contains(response.Body.String(), `"status":"copying"`) {
		t.Fatalf("migration status response: %d %s", response.Code, response.Body.String())
	}
}

func TestAssistantRuntimeMigrationCopiesThenCleansLegacyRuntime(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	encryptedKey, err := crypto.Encrypt(key, "provider-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := &model.AssistantProvider{Name: "Responses", ProviderType: assistantProviderTypeResponses, Model: "gpt-4.1-mini", APIKeyEncrypted: encryptedKey, Enabled: true}
	if err := st.CreateAssistantProvider(provider); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "worker-a", Host: "worker-a.example.test", SSHAuthType: "key", K8sNodeName: "worker-a"}); err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	legacyClaim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent-audit", Namespace: "default"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "legacy-audit-pv", Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	targetClaim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent-audit", Namespace: "cylism-assistant"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "target-audit-pv", Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	volume := func(name, path string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}},
				NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"worker-a"},
					}}}},
				}},
			},
		}
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		legacyClaim,
		targetClaim,
		volume("legacy-audit-pv", "/var/lib/rancher/k3s/storage/legacy"),
		volume("target-audit-pv", "/var/lib/rancher/k3s/storage/target"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}}},
			},
		},
	)}
	defer func() { K8s = originalK8s }()
	migration := &model.AssistantRuntimeMigration{ProviderID: provider.ID, SourceNamespace: "default", SourcePVCName: "cylism-ops-agent-audit", SourceNodeName: "worker-a", SourceReplicas: 1, TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit", TargetNodeName: "worker-a", Storage: "1Gi", Status: model.AssistantRuntimeMigrationPending}
	if err := st.CreateAssistantRuntimeMigration(migration); err != nil {
		t.Fatal(err)
	}
	handler := NewAssistantHandler(st, key)
	handler.runtimeMigrationHooks = &assistantRuntimeMigrationHooks{
		preflight: func(*model.Server, string) error { return nil },
		transfer:  func(*model.Server, *model.Server, string, string, []byte) (int64, error) { return 256, nil },
		checksums: func(*model.Server, string) (string, error) { return "audit.db:checksum", nil },
		waitPods:  func(string, string, bool, time.Duration) error { return nil },
		waitReady: func(time.Duration) error { return nil },
	}
	handler.runAssistantRuntimeMigration(migration.ID)
	updated, err := st.GetAssistantRuntimeMigration(migration.ID)
	if err != nil || updated.Status != model.AssistantRuntimeMigrationSucceeded || updated.BytesCopied != 256 {
		t.Fatalf("migration = %#v, %v", updated, err)
	}
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims("default").Get(K8s.Ctx(), "cylism-ops-agent-audit", metav1.GetOptions{}); err == nil {
		t.Fatal("legacy PVC was not cleaned after a verified cutover")
	}
	if _, err := K8s.Clientset.AppsV1().Deployments("cylism-assistant").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{}); err != nil {
		t.Fatalf("target Runtime was not deployed: %v", err)
	}
}

func TestAssistantInstallRejectsActiveRuntimeNodeChange(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	encryptedKey, err := crypto.Encrypt(key, "provider-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := &model.AssistantProvider{Name: "Responses", ProviderType: assistantProviderTypeResponses, Model: "gpt-4.1-mini", APIKeyEncrypted: encryptedKey, Enabled: true}
	if err := st.CreateAssistantProvider(provider); err != nil {
		t.Fatal(err)
	}
	claimName := "cylism-ops-agent-audit"
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "cylism-assistant"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "audit-pv", Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "audit-pv"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/rancher/k3s/storage/audit"}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"worker-a"}}}}}}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "cylism-assistant"}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: "worker-a"}, Volumes: []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName}}}}}}}},
	)}
	defer func() { K8s = originalK8s }()

	g := gin.New()
	g.POST("/api/assistant/install", NewAssistantHandler(st, key).Install)
	response := serve(g, newJSONRequest(http.MethodPost, "/api/assistant/install", gin.H{"provider_id": provider.ID, "node_name": "worker-b", "storage": "1Gi"}))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "迁移 Runtime 存储") {
		t.Fatalf("install response: %d %s", response.Code, response.Body.String())
	}
}

func TestAssistantRuntimeNodeMigrationSwitchesDeploymentAndCleansSourcePVC(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "worker-a", Host: "worker-a.example.test", SSHAuthType: "key", K8sNodeName: "worker-a"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateServer(&model.Server{Name: "worker-b", Host: "worker-b.example.test", SSHAuthType: "key", K8sNodeName: "worker-b"}); err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	sourceName, targetName := "cylism-ops-agent-audit", "cylism-ops-agent-audit-migrate-42"
	claim := func(name, volume string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "cylism-assistant", Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/infrastructure": "ops-agent", "cylism.io/component": "assistant"}}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: volume, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	}
	volume := func(name, node, path string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{node}}}}}}}}}
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		claim(sourceName, "source-pv"), claim(targetName, "target-pv"), volume("source-pv", "worker-a", "/data/source"), volume("target-pv", "worker-b", "/data/target"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "cylism-assistant", Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager"}},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "worker-a"},
				Volumes:      []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: sourceName}}}},
			}}},
		},
	)}
	defer func() { K8s = originalK8s }()
	migration := &model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "cylism-assistant", SourcePVCName: sourceName, SourceNodeName: "worker-a", SourceReplicas: 1, TargetNamespace: "cylism-assistant", TargetPVCName: targetName, TargetNodeName: "worker-b", Storage: "1Gi", Status: model.AssistantRuntimeMigrationPending}
	if err := st.CreateAssistantRuntimeMigration(migration); err != nil {
		t.Fatal(err)
	}
	handler := NewAssistantHandler(st, nil)
	handler.runtimeMigrationHooks = &assistantRuntimeMigrationHooks{preflight: func(*model.Server, string) error { return nil }, transfer: func(*model.Server, *model.Server, string, string, []byte) (int64, error) { return 512, nil }, checksums: func(*model.Server, string) (string, error) { return "audit.db:checksum", nil }, waitPods: func(string, string, bool, time.Duration) error { return nil }, waitReady: func(time.Duration) error { return nil }}
	handler.runAssistantRuntimeMigration(migration.ID)
	updated, err := st.GetAssistantRuntimeMigration(migration.ID)
	if err != nil || updated.Status != model.AssistantRuntimeMigrationSucceeded || updated.BytesCopied != 512 {
		t.Fatalf("migration = %#v, %v", updated, err)
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("cylism-assistant").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName; got != targetName || deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] != "worker-b" {
		t.Fatalf("unexpected Runtime cutover: %#v", deployment.Spec.Template.Spec)
	}
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims("cylism-assistant").Get(K8s.Ctx(), sourceName, metav1.GetOptions{}); err == nil {
		t.Fatal("source audit PVC was not deleted after successful node migration")
	}
}

func TestAssistantRuntimeNodeMigrationCopyFailureRestoresSourceRuntime(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{"worker-a", "worker-b"} {
		if err := st.CreateServer(&model.Server{Name: node, Host: node + ".example.test", SSHAuthType: "key", K8sNodeName: node}); err != nil {
			t.Fatal(err)
		}
	}
	replicas := int32(1)
	sourceName, targetName := "cylism-ops-agent-audit", "cylism-ops-agent-audit-migrate-failure"
	claim := func(name, volume string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "cylism-assistant", Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/infrastructure": "ops-agent", "cylism.io/component": "assistant"}}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: volume, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	}
	volume := func(name, node, path string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{node}}}}}}}}}
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		claim(sourceName, "source-pv"), claim(targetName, "target-pv"), volume("source-pv", "worker-a", "/data/source"), volume("target-pv", "worker-b", "/data/target"),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cylism-ops-agent", Namespace: "cylism-assistant", Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager"}},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				NodeSelector: map[string]string{corev1.LabelHostname: "worker-a"},
				Volumes:      []corev1.Volume{{Name: "audit", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: sourceName}}}},
			}}},
		},
	)}
	defer func() { K8s = originalK8s }()
	migration := &model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "cylism-assistant", SourcePVCName: sourceName, SourceNodeName: "worker-a", SourceReplicas: 1, TargetNamespace: "cylism-assistant", TargetPVCName: targetName, TargetNodeName: "worker-b", Storage: "1Gi", Status: model.AssistantRuntimeMigrationPending}
	if err := st.CreateAssistantRuntimeMigration(migration); err != nil {
		t.Fatal(err)
	}
	handler := NewAssistantHandler(st, nil)
	handler.runtimeMigrationHooks = &assistantRuntimeMigrationHooks{preflight: func(*model.Server, string) error { return nil }, transfer: func(*model.Server, *model.Server, string, string, []byte) (int64, error) {
		return 0, fmt.Errorf("transfer failed")
	}, waitPods: func(string, string, bool, time.Duration) error { return nil }}
	handler.runAssistantRuntimeMigration(migration.ID)
	updated, err := st.GetAssistantRuntimeMigration(migration.ID)
	if err != nil || updated.Status != model.AssistantRuntimeMigrationFailed {
		t.Fatalf("migration = %#v, %v", updated, err)
	}
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims("cylism-assistant").Get(K8s.Ctx(), sourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("source PVC must be retained after copy failure: %v", err)
	}
	if _, err := K8s.Clientset.CoreV1().PersistentVolumeClaims("cylism-assistant").Get(K8s.Ctx(), targetName, metav1.GetOptions{}); err == nil {
		t.Fatal("failed migration target PVC was not cleaned")
	}
	deployment, err := K8s.Clientset.AppsV1().Deployments("cylism-assistant").Get(K8s.Ctx(), "cylism-ops-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if *deployment.Spec.Replicas != 1 || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != sourceName {
		t.Fatalf("source Runtime was not restored: %#v", deployment.Spec)
	}
}

func TestAssistantRuntimeNodeMigrationRejectsDuplicateActiveMigration(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAssistantRuntimeMigration(&model.AssistantRuntimeMigration{ProviderID: 1, SourceNamespace: "cylism-assistant", SourcePVCName: "cylism-ops-agent-audit", TargetNamespace: "cylism-assistant", TargetPVCName: "cylism-ops-agent-audit-migrate-1", TargetNodeName: "worker-b", Storage: "1Gi", Status: model.AssistantRuntimeMigrationCopying}); err != nil {
		t.Fatal(err)
	}
	originalK8s := K8s
	K8s = &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	defer func() { K8s = originalK8s }()

	g := gin.New()
	g.POST("/api/assistant/runtime/migrations", NewAssistantHandler(st, nil).MigrateRuntimeNode)
	response := serve(g, newJSONRequest(http.MethodPost, "/api/assistant/runtime/migrations", gin.H{"target_node_name": "worker-b"}))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "已有 Runtime 存储迁移") {
		t.Fatalf("migration response: %d %s", response.Code, response.Body.String())
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
