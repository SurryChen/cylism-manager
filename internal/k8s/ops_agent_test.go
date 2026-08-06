package k8s

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOpsAgentConfigUsesResponsesModelName(t *testing.T) {
	client := &Client{Clientset: fake.NewSimpleClientset()}
	if err := client.upsertOpsAgentConfig(OpsAgentConfig{Model: "gpt-4o-mini", BaseURL: "https://responses.example.test/v1"}); err != nil {
		t.Fatal(err)
	}

	configMap, err := client.Clientset.CoreV1().ConfigMaps(opsAgentNamespace).Get(client.Ctx(), opsAgentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := configMap.Data["OPS_AGENT_MODEL"]; got != "gpt-4o-mini" {
		t.Fatalf("OPS_AGENT_MODEL = %q, want bare Responses API model name", got)
	}
	if got := configMap.Data["OPENAI_BASE_URL"]; got != "https://responses.example.test/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q, want configured Responses API endpoint", got)
	}
}
