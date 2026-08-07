package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
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
}

type Adapter interface {
	Definition() Definition
	Validate(instance *model.RuntimeInstance) error
	Config(instance *model.RuntimeInstance) (string, error)
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
		DefaultPort:             RuntimeDefaultPort,
		DefaultHealthPath:       "/health",
		SupportedModelProtocols: []string{ModelProtocolResponses, ModelProtocolAnthropic},
	}
}

func (a NanobotAdapter) Validate(instance *model.RuntimeInstance) error {
	if instance.APIStyle == "" {
		instance.APIStyle = ModelProtocolResponses
	}
	for _, protocol := range a.Definition().SupportedModelProtocols {
		if instance.APIStyle == protocol {
			return nil
		}
	}
	return fmt.Errorf("Runtime %q 不支持模型协议 %q", instance.RuntimeType, instance.APIStyle)
}

func (NanobotAdapter) Config(instance *model.RuntimeInstance) (string, error) {
	raw := map[string]interface{}{}
	if strings.TrimSpace(instance.Config) != "" {
		if err := json.Unmarshal([]byte(instance.Config), &raw); err != nil {
			return "", fmt.Errorf("Runtime 配置不是合法 JSON: %w", err)
		}
	}
	if instance.ModelName != "" {
		raw["model"] = instance.ModelName
	}
	if instance.ModelBaseURL != "" {
		raw["base_url"] = instance.ModelBaseURL
	}
	if instance.APIStyle != "" {
		raw["model_protocol"] = instance.APIStyle
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("编码 Runtime 配置: %w", err)
	}
	return string(encoded), nil
}
