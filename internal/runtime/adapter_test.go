package runtime

import (
	"encoding/json"
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

func (testAdapter) Workload(instance *model.RuntimeInstance) (WorkloadSpec, error) {
	return WorkloadSpec{}, nil
}

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
	instance := &model.RuntimeInstance{RuntimeType: model.RuntimeTypeNanobot, Image: "example/nanobot:latest", APIStyle: ModelProtocolResponses, ModelName: "qwen-max", ModelBaseURL: "https://provider.example/v1"}
	if err := adapter.Validate(instance); err != nil {
		t.Fatalf("responses protocol should be supported: %v", err)
	}
	config, err := adapter.Config(instance)
	if err != nil {
		t.Fatalf("generate responses config: %v", err)
	}
	if !strings.Contains(config, `"apiType":"responses"`) || !strings.Contains(config, `"apiKey":"${CYLISM_MODEL_API_KEY}"`) {
		t.Fatalf("unexpected responses config: %s", config)
	}
	var native map[string]any
	if err := json.Unmarshal([]byte(config), &native); err != nil {
		t.Fatalf("decode native config: %v", err)
	}
	tools := native["tools"].(map[string]any)
	if tools["restrictToWorkspace"] != true || tools["webuiAllowRemotePackageInstall"] != false || tools["exec"].(map[string]any)["enable"] != false {
		t.Fatalf("unsafe native tool policy: %#v", tools)
	}
	if native["api"].(map[string]any)["apiKey"] != "${CYLISM_RUNTIME_API_KEY}" {
		t.Fatalf("runtime API credential is not distinct: %#v", native["api"])
	}
	workload, err := adapter.Workload(instance)
	if err != nil {
		t.Fatalf("generate workload: %v", err)
	}
	if len(workload.InitContainers) != 1 || len(workload.Containers) != 2 || workload.ServicePort != 8900 || workload.HealthPort != 8900 {
		t.Fatalf("unexpected nanobot workload: %#v", workload)
	}
	if workload.Containers[0].Name != "gateway" || workload.Containers[1].Name != "api" {
		t.Fatalf("unexpected nanobot container names: %#v", workload.Containers)
	}

	instance.APIStyle = ModelProtocolAnthropic
	config, err = adapter.Config(instance)
	if err != nil || !strings.Contains(config, `"anthropic"`) || !strings.Contains(config, `"apiBase":"https://provider.example/v1"`) {
		t.Fatalf("unexpected anthropic config: %s err=%v", config, err)
	}
	instance.APIStyle = "openai-chat"
	if err := adapter.Validate(instance); err == nil {
		t.Fatal("unsupported protocol should fail validation")
	}
}
