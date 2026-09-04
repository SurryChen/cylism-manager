package shared

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildDetailRedactsNestedReleaseSecrets(t *testing.T) {
	detail := buildDetail("POST", "/api/applications/1/releases", []byte(`{"image":"nginx:1.27","secrets":{"DATABASE_PASSWORD":"do-not-log"},"token":"also-private"}`), nil)
	if strings.Contains(detail, "do-not-log") || strings.Contains(detail, "also-private") || !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("audit detail leaked sensitive data: %s", detail)
	}
}

func TestRedactConfigMapContent(t *testing.T) {
	value := map[string]interface{}{"content": "private-config", "expected_revision": float64(1)}
	encoded, _ := json.Marshal(redactAuditValue(value))
	if strings.Contains(string(encoded), "private-config") || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("ConfigMap content leaked in audit detail: %s", encoded)
	}
}
