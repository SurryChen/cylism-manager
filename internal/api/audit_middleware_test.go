package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildDetailRedactsNestedReleaseSecrets(t *testing.T) {
	detail := buildDetail("POST", "/api/applications/1/releases", []byte(`{"image":"nginx:1.27","secrets":{"DATABASE_PASSWORD":"do-not-log"},"token":"also-private"}`), nil)
	if strings.Contains(detail, "do-not-log") || strings.Contains(detail, "also-private") {
		t.Fatalf("audit detail leaked a sensitive value: %s", detail)
	}
	if !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("expected redacted marker: %s", detail)
	}
}

func TestRedactManagedFileContent(t *testing.T) {
	value := map[string]interface{}{"content": "private-config", "expected_version": float64(1)}
	redacted := redactManagedFileContent(value).(map[string]interface{})
	encoded, _ := json.Marshal(redacted)
	if strings.Contains(string(encoded), "private-config") || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("managed file content leaked in audit detail: %s", encoded)
	}
}
