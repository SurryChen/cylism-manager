package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ModelProtocolResponses = "responses"
	ModelProtocolAnthropic = "anthropic"
)

// Definition is the platform-facing contract for one Agent Runtime family.
// The Runtime itself owns session and memory semantics; the adapter only
// describes deployment and model-protocol capabilities.
type Definition struct {
	RuntimeType             string   `json:"runtime_type"`
	DisplayName             string   `json:"display_name"`
	Description             string   `json:"description"`
	DefaultPort             int32    `json:"default_port"`
	DefaultHealthPath       string   `json:"default_health_path"`
	SupportedModelProtocols []string `json:"supported_model_protocols"`
	Capabilities            []string `json:"capabilities,omitempty"`
}

type Adapter interface {
	Definition() Definition
	Validate(instance *model.RuntimeInstance) error
	Config(instance *model.RuntimeInstance) (string, error)
	Workload(instance *model.RuntimeInstance) (WorkloadSpec, error)
	ChatEndpoint(instance *model.RuntimeInstance) (string, error)
	SessionEndpoint(instance *model.RuntimeInstance) (string, error)
}

// WorkloadSpec is a platform-owned Kubernetes workload description. Adapter
// inputs never become arbitrary Pod fields, keeping runtime configuration and
// Kubernetes execution privileges separate.
type WorkloadSpec struct {
	InitContainers []corev1.Container
	Containers     []corev1.Container
	ServicePort    int32
	HealthPort     int32
	HealthPath     string
	SessionPort    int32
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: make(map[string]Adapter)}
	for _, adapter := range adapters {
		registry.Register(adapter)
	}
	return registry
}

func BuiltinRegistry() *Registry {
	return NewRegistry(NanobotAdapter{})
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("Runtime Adapter 不能为空")
	}
	definition := adapter.Definition()
	if strings.TrimSpace(definition.RuntimeType) == "" {
		return fmt.Errorf("Runtime Adapter 类型不能为空")
	}
	if r.adapters == nil {
		r.adapters = make(map[string]Adapter)
	}
	if _, exists := r.adapters[definition.RuntimeType]; exists {
		return fmt.Errorf("Runtime Adapter %q 已注册", definition.RuntimeType)
	}
	r.adapters[definition.RuntimeType] = adapter
	return nil
}

func (r *Registry) Get(runtimeType string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.adapters[strings.TrimSpace(runtimeType)]
	return adapter, ok
}

func (r *Registry) List() []Definition {
	definitions := make([]Definition, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		definitions = append(definitions, adapter.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].RuntimeType < definitions[j].RuntimeType })
	return definitions
}

func (r *Registry) Validate(instance *model.RuntimeInstance) error {
	adapter, ok := r.Get(instance.RuntimeType)
	if !ok {
		return fmt.Errorf("不支持的 Runtime 类型: %s", instance.RuntimeType)
	}
	return adapter.Validate(instance)
}

type NanobotAdapter struct{}

func (NanobotAdapter) Definition() Definition {
	return Definition{
		RuntimeType:             model.RuntimeTypeNanobot,
		DisplayName:             "nanobot",
		Description:             "拥有会话、长期记忆和 Gateway 的外部 Agent Runtime",
		DefaultPort:             NanobotAPIPort,
		DefaultHealthPath:       "/health",
		SupportedModelProtocols: []string{ModelProtocolResponses, ModelProtocolAnthropic},
		Capabilities:            []string{"chat", "sessions"},
	}
}

func (a NanobotAdapter) Validate(instance *model.RuntimeInstance) error {
	if instance.APIStyle == "" {
		instance.APIStyle = ModelProtocolResponses
	}
	for _, protocol := range a.Definition().SupportedModelProtocols {
		if instance.APIStyle == protocol {
			_, err := a.Config(instance)
			return err
		}
	}
	return fmt.Errorf("Runtime %q 不支持模型协议 %q", instance.RuntimeType, instance.APIStyle)
}

func (NanobotAdapter) Config(instance *model.RuntimeInstance) (string, error) {
	if strings.TrimSpace(instance.Config) != "" && strings.TrimSpace(instance.Config) != "{}" {
		var configured map[string]interface{}
		if err := json.Unmarshal([]byte(instance.Config), &configured); err != nil {
			return "", fmt.Errorf("Runtime 配置不是合法 JSON: %w", err)
		}
		if len(configured) > 0 {
			return "", fmt.Errorf("Nanobot Runtime 暂不支持覆盖平台管理的原生配置")
		}
	}
	provider := "openai"
	providers := map[string]interface{}{}
	if instance.APIStyle == ModelProtocolAnthropic {
		provider = "anthropic"
		providers[provider] = map[string]interface{}{
			"apiKey":  "${CYLISM_MODEL_API_KEY}",
			"apiBase": instance.ModelBaseURL,
		}
	} else {
		providers[provider] = map[string]interface{}{
			"apiKey":  "${CYLISM_MODEL_API_KEY}",
			"apiBase": instance.ModelBaseURL,
			"apiType": ModelProtocolResponses,
		}
	}
	raw := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace": "/data/workspace",
				"model":     instance.ModelName,
				"provider":  provider,
			},
		},
		"providers": providers,
		"gateway": map[string]interface{}{
			"host": "0.0.0.0",
			"port": NanobotGatewayPort,
		},
		"api": map[string]interface{}{
			"host":   "0.0.0.0",
			"port":   NanobotAPIPort,
			"apiKey": "${CYLISM_RUNTIME_API_KEY}",
		},
		"tools": map[string]interface{}{
			"exec":                           map[string]interface{}{"enable": false},
			"restrictToWorkspace":            true,
			"webuiAllowRemotePackageInstall": false,
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("编码 Runtime 配置: %w", err)
	}
	return string(encoded), nil
}

// ChatEndpoint returns the runtime's OpenAI-compatible chat URL.
func (NanobotAdapter) ChatEndpoint(instance *model.RuntimeInstance) (string, error) {
	if instance == nil || strings.TrimSpace(instance.Name) == "" || strings.TrimSpace(instance.Namespace) == "" {
		return "", fmt.Errorf("Runtime 缺少名称或命名空间")
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/v1/chat/completions", instance.Name, instance.Namespace, NanobotAPIPort), nil
}

// SessionEndpoint returns the runtime's read-only session API URL.
func (NanobotAdapter) SessionEndpoint(instance *model.RuntimeInstance) (string, error) {
	if instance == nil || strings.TrimSpace(instance.Name) == "" || strings.TrimSpace(instance.Namespace) == "" {
		return "", fmt.Errorf("Runtime 缺少名称或命名空间")
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", instance.Name, instance.Namespace, NanobotSessionPort), nil
}

func (NanobotAdapter) Workload(instance *model.RuntimeInstance) (WorkloadSpec, error) {
	if strings.TrimSpace(instance.Image) == "" {
		return WorkloadSpec{}, fmt.Errorf("Nanobot Runtime 镜像不能为空")
	}
	probe := func(port int32) *corev1.Probe {
		return &corev1.Probe{
			ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(port)}},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      3,
			FailureThreshold:    6,
		}
	}
	gateway := corev1.Container{
		Name:           "gateway",
		Image:          instance.Image,
		Command:        []string{"nanobot", "gateway", "--foreground", "--config", NanobotConfigPath},
		Ports:          []corev1.ContainerPort{{Name: "gateway", ContainerPort: NanobotGatewayPort}},
		ReadinessProbe: probe(NanobotGatewayPort),
		LivenessProbe:  probe(NanobotGatewayPort),
	}
	api := corev1.Container{
		Name:           "api",
		Image:          instance.Image,
		Command:        []string{"nanobot", "serve", "--host", "0.0.0.0", "--port", fmt.Sprintf("%d", NanobotAPIPort), "--config", NanobotConfigPath},
		Ports:          []corev1.ContainerPort{{Name: "api", ContainerPort: NanobotAPIPort}},
		ReadinessProbe: probe(NanobotAPIPort),
		LivenessProbe:  probe(NanobotAPIPort),
	}
	sessionAPI := corev1.Container{
		Name:           "session-api",
		Image:          instance.Image,
		Command:        []string{"cylism-session-api"},
		Ports:          []corev1.ContainerPort{{Name: "session", ContainerPort: NanobotSessionPort}},
		ReadinessProbe: probe(NanobotSessionPort),
		LivenessProbe:  probe(NanobotSessionPort),
	}
	return WorkloadSpec{
		InitContainers: []corev1.Container{{
			Name:    "render-config",
			Image:   instance.Image,
			Command: []string{"cylism-render-nanobot-config", "--source", "/etc/cylism/runtime.json", "--destination", NanobotConfigPath},
		}},
		Containers:  []corev1.Container{gateway, api, sessionAPI},
		ServicePort: NanobotAPIPort,
		HealthPort:  NanobotAPIPort,
		HealthPath:  "/health",
		SessionPort: NanobotSessionPort,
	}, nil
}
