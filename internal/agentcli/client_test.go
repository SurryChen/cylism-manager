package agentcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRunCapabilityStatusUsesFixedAgentEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/agent/v1/capabilities/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"data":{"cluster.read":{"enabled":true}},"summary":"capability status retrieved"}`))
	}))
	defer server.Close()
	output := &bytes.Buffer{}
	code := Run(context.Background(), []string{"capability", "status", "--output", "json"}, Config{BaseURL: server.URL, TokenFile: writeToken(t, "runtime-token")}, output)
	if code != 0 {
		t.Fatalf("expected success, got %d: %s", code, output.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || envelope.Status != StatusOK {
		t.Fatalf("unexpected envelope: %#v err=%v", envelope, err)
	}
}

func TestRunPendingPodDiagnosticCommandsUseFixedEndpoints(t *testing.T) {
	tests := []struct {
		args  []string
		path  string
		query string
	}{
		{[]string{"pod", "get", "--namespace", "kube-system", "--name", "local-path-provisioner", "--output", "json"}, "/api/agent/v1/pods/get", "name=local-path-provisioner&namespace=kube-system"},
		{[]string{"event", "list", "--namespace", "kube-system", "--involved-kind", "pod", "--involved-name", "local-path-provisioner", "--limit", "20", "--output", "json"}, "/api/agent/v1/events/list", "involved_kind=pod&involved_name=local-path-provisioner&limit=20&namespace=kube-system"},
		{[]string{"pvc", "get", "--namespace", "kube-system", "--name", "data", "--output", "json"}, "/api/agent/v1/pvcs/get", "name=data&namespace=kube-system"},
		{[]string{"node", "get", "--name", "node-1", "--output", "json"}, "/api/agent/v1/nodes/get", "name=node-1"},
	}
	for _, test := range tests {
		t.Run(test.args[0]+" "+test.args[1], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != test.path || r.URL.RawQuery != test.query {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				_, _ = w.Write([]byte(`{"status":"ok","summary":"diagnostic retrieved"}`))
			}))
			defer server.Close()
			output := &bytes.Buffer{}
			if code := Run(context.Background(), test.args, Config{BaseURL: server.URL, TokenFile: writeToken(t, "runtime-token")}, output); code != 0 {
				t.Fatalf("expected success, got %d: %s", code, output.String())
			}
		})
	}
}

func TestRunRegistryDiagnosticCommandsUseFixedEndpoints(t *testing.T) {
	tests := []struct {
		args, path, query string
		method            string
	}{
		{"registry status --output json", "/api/agent/v1/registries/status", "", http.MethodGet},
		{"image diagnose --namespace kube-system --pod pending-pod --output json", "/api/agent/v1/images/diagnose", "namespace=kube-system&pod=pending-pod", http.MethodGet},
		{"registry node-verify --node node-1 --registry registry.k8s.io --output json", "/api/agent/v1/registries/node-verify", "node=node-1&registry=registry.k8s.io", http.MethodGet},
		{"registry node-pull-check --node node-1 --registry registry.k8s.io --output json", "/api/agent/v1/registries/node-pull-check", "", http.MethodPost},
		{"registry proxy-diagnose --registry docker.io --output json", "/api/agent/v1/registries/proxy-diagnose", "registry=docker.io", http.MethodGet},
		{"dns status --output json", "/api/agent/v1/dns/status", "", http.MethodGet},
		{"dns resolve --name registry-1.docker.io --output json", "/api/agent/v1/dns/resolve", "name=registry-1.docker.io", http.MethodGet},
	}
	for _, test := range tests {
		t.Run(test.args, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != test.method || r.URL.Path != test.path {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				if test.method == http.MethodGet && r.URL.RawQuery != test.query {
					t.Fatalf("unexpected query: %s", r.URL.RawQuery)
				}
				if test.method == http.MethodPost {
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["node"] != "node-1" || body["registry"] != "registry.k8s.io" {
						t.Fatalf("unexpected body: %#v", body)
					}
				}
				_, _ = w.Write([]byte(`{"status":"ok","summary":"registry diagnostic retrieved"}`))
			}))
			defer server.Close()
			output := &bytes.Buffer{}
			if code := Run(context.Background(), strings.Fields(test.args), Config{BaseURL: server.URL, TokenFile: writeToken(t, "runtime-token")}, output); code != 0 {
				t.Fatalf("expected success, got %d: %s", code, output.String())
			}
		})
	}
}

func TestRunAlertAutomationCommandsUseFixedEndpoints(t *testing.T) {
	tests := []struct {
		args, path, query, method string
	}{
		{"alert list --output json", "/api/agent/v1/alerts/list", "", http.MethodGet},
		{"alert get --id 42 --output json", "/api/agent/v1/alerts/get", "id=42", http.MethodGet},
		{"monitoring disk-growth --node node-1 --range 6h --output json", "/api/agent/v1/monitoring/disk-growth", "node=node-1&range=6h", http.MethodGet},
		{"maintenance disk-inspect --node node-1 --output json", "/api/agent/v1/maintenance/disk-inspect", "node=node-1", http.MethodGet},
		{"maintenance cleanup-request --alert 42 --recipe journal-vacuum --output json", "/api/agent/v1/maintenance/cleanup-request", "", http.MethodPost},
	}
	for _, test := range tests {
		t.Run(test.args, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != test.method || r.URL.Path != test.path || r.URL.RawQuery != test.query {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				if test.method == http.MethodPost {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["alert_id"] != float64(42) || body["recipe"] != "journal-vacuum" {
						t.Fatalf("unexpected cleanup request: %#v err=%v", body, err)
					}
				}
				_, _ = w.Write([]byte(`{"status":"ok","summary":"request completed"}`))
			}))
			defer server.Close()
			if code := Run(context.Background(), strings.Fields(test.args), Config{BaseURL: server.URL, TokenFile: writeToken(t, "runtime-token")}, &bytes.Buffer{}); code != 0 {
				t.Fatalf("expected success, got %d", code)
			}
		})
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
