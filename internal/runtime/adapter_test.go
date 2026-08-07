package runtime

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type testAdapter struct{}

func (testAdapter) Definition() Definition {
	return Definition{RuntimeType: "test-agent", DisplayName: "Test Agent", SupportedModelProtocols: []string{"custom"}}
}

func (testAdapter) Validate(instance *model.RuntimeInstance) error { return nil }

func (testAdapter) Config(instance *model.RuntimeInstance) (string, error) { return "{}", nil }

func TestRegistryListsAndValidatesRegisteredAdapters(t *testing.T) {
	registry := NewRegistry(NanobotAdapter{})
	if err := registry.Register(testAdapter{}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	definitions := registry.List()
	if len(definitions) != 2 || definitions[0].RuntimeType != model.RuntimeTypeNanobot || definitions[1].RuntimeType != "test-agent" {
		t.Fatalf("unexpected registry definitions: %#v", definitions)
	}
	if _, ok := registry.Get("test-agent"); !ok {
		t.Fatal("registered adapter not found")
	}
	if err := registry.Register(testAdapter{}); err == nil {
		t.Fatal("duplicate adapter registration should fail")
	}
}

func TestNanobotAdapterDeclaresAndValidatesModelProtocols(t *testing.T) {
	adapter := NanobotAdapter{}
	instance := &model.RuntimeInstance{RuntimeType: model.RuntimeTypeNanobot, APIStyle: ModelProtocolResponses, ModelName: "qwen-max", ModelBaseURL: "https://provider.example/v1"}
	if err := adapter.Validate(instance); err != nil {
		t.Fatalf("responses protocol should be supported: %v", err)
	}
	config, err := adapter.Config(instance)
	if err != nil || !strings.Contains(config, `"model_protocol":"responses"`) {
		t.Fatalf("unexpected adapter config: %s err=%v", config, err)
	}
	instance.APIStyle = "openai-chat"
	if err := adapter.Validate(instance); err == nil {
		t.Fatal("unsupported protocol should fail validation")
	}
}
