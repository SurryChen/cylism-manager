package agentcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunClusterStatusUsesFixedAgentEndpointAndJSONEnvelope(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if r.Method != http.MethodGet || r.URL.Path != "/api/agent/v1/cluster/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"data":{"ready":true},"summary":"cluster is ready"}`))
	}))
	defer server.Close()

	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"cluster", "status", "--output", "json"}, Config{
		BaseURL:    server.URL,
		TokenFile:  writeToken(t, "runtime-token"),
		HTTPClient: &http.Client{Timeout: time.Second},
	}, output)
	if code != 0 {
		t.Fatalf("expected success, got %d: %s", code, output.String())
	}
	if authorization != "Bearer runtime-token" {
		t.Fatalf("unexpected authorization header: %q", authorization)
	}
	var envelope Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope.Status != StatusOK || envelope.Summary != "cluster is ready" || envelope.RequestID == "" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestRunDeploymentScaleHasIdempotencyKeyAndExactBody(t *testing.T) {
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idempotencyKey = r.Header.Get("Idempotency-Key")
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/v1/deployments/scale" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["namespace"] != "operations" || body["name"] != "api" || body["replicas"] != float64(3) {
			t.Fatalf("unexpected body: %#v", body)
		}
		_, _ = w.Write([]byte(`{"status":"pending_approval","operation_id":"op-1","summary":"approval required"}`))
	}))
	defer server.Close()

	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"deployment", "scale", "--namespace", "operations", "--name", "api", "--replicas", "3", "--output", "json"}, Config{
		BaseURL:    server.URL,
		TokenFile:  writeToken(t, "runtime-token"),
		HTTPClient: &http.Client{Timeout: time.Second},
	}, output)
	if code != 0 {
		t.Fatalf("expected pending approval to be a protocol success, got %d: %s", code, output.String())
	}
	if idempotencyKey == "" {
		t.Fatal("expected idempotency key")
	}
	var envelope Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope.Status != StatusPendingApproval || envelope.OperationID != "op-1" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestRunPreservesManagerErrorEnvelopeForHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":"error","summary":"capability not granted","retryable":false}`))
	}))
	defer server.Close()

	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"cluster", "status", "--output", "json"}, Config{
		BaseURL:    server.URL,
		TokenFile:  writeToken(t, "runtime-token"),
		HTTPClient: &http.Client{Timeout: time.Second},
	}, output)
	if code == 0 {
		t.Fatalf("expected failure, got %d: %s", code, output.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope.Status != StatusError || envelope.Summary != "capability not granted" || envelope.Retryable || envelope.RequestID == "" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestRunUsesHTTPStatusWhenManagerErrorBodyIsNotJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream unavailable"))
	}))
	defer server.Close()

	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"cluster", "status", "--output", "json"}, Config{
		BaseURL:    server.URL,
		TokenFile:  writeToken(t, "runtime-token"),
		HTTPClient: &http.Client{Timeout: time.Second},
	}, output)
	if code == 0 {
		t.Fatalf("expected failure, got %d: %s", code, output.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if envelope.Summary != "agent API rejected the request (HTTP 502)" || !envelope.Retryable {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestRunRejectsUnknownOrFreeformCommandsBeforeNetworkRequest(t *testing.T) {
	for _, arguments := range [][]string{
		{"request", "GET", "https://example.invalid", "--output", "json"},
		{"cluster", "status", "--url", "https://example.invalid", "--output", "json"},
	} {
		output := &bytes.Buffer{}
		code := Run(context.Background(), arguments, Config{BaseURL: "http://127.0.0.1:1", TokenFile: writeToken(t, "secret")}, output)
		if code == 0 {
			t.Fatalf("expected rejection for %q", arguments)
		}
		if bytes.Contains(output.Bytes(), []byte("secret")) {
			t.Fatalf("credential leaked in output: %s", output.String())
		}
		var envelope Envelope
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if envelope.Status != StatusError {
			t.Fatalf("unexpected envelope: %#v", envelope)
		}
	}
}

func TestRunRejectsMissingOrOversizedTokenWithoutSendingIt(t *testing.T) {
	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"cluster", "status", "--output", "json"}, Config{
		BaseURL:   "http://127.0.0.1:1",
		TokenFile: writeToken(t, string(bytes.Repeat([]byte("a"), maxTokenBytes+1))),
	}, output)
	if code == 0 || bytes.Contains(output.Bytes(), []byte("aaaa")) {
		t.Fatalf("expected safe token rejection, got %d: %s", code, output.String())
	}
}

func writeToken(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return path
}
