// Package agentcli implements the fixed, JSON-only Runtime-to-Manager client.
package agentcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultTimeout   = 15 * time.Second
	maxTokenBytes    = 16 * 1024
	maxResponseBytes = 1024 * 1024
)

var resourceNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]{0,251}[a-z0-9])?$`)

type Status string

const (
	StatusOK              Status = "ok"
	StatusPendingApproval Status = "pending_approval"
	StatusError           Status = "error"
)

// Envelope is the only output contract exposed to the Runtime tool adapter.
type Envelope struct {
	Status      Status          `json:"status"`
	RequestID   string          `json:"request_id"`
	OperationID string          `json:"operation_id,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	Summary     string          `json:"summary"`
	Retryable   bool            `json:"retryable"`
}

type Config struct {
	BaseURL    string
	TokenFile  string
	HTTPClient *http.Client
}

type requestSpec struct {
	method   string
	path     string
	query    url.Values
	body     any
	mutation bool
}

type agentResponse struct {
	Status      Status          `json:"status"`
	OperationID string          `json:"operation_id"`
	Data        json.RawMessage `json:"data"`
	Summary     string          `json:"summary"`
	Retryable   bool            `json:"retryable"`
}

// Run validates a registered command, makes one bounded Manager request, and writes one JSON envelope.
func Run(ctx context.Context, args []string, cfg Config, output io.Writer) int {
	requestID := uuid.NewString()
	spec, err := parseCommand(args)
	if err != nil {
		writeEnvelope(output, errorEnvelope(requestID, err.Error(), false))
		return 2
	}
	baseURL, err := parseBaseURL(cfg.BaseURL)
	if err != nil {
		writeEnvelope(output, errorEnvelope(requestID, "agent API is not configured", false))
		return 2
	}
	token, err := readToken(cfg.TokenFile)
	if err != nil {
		writeEnvelope(output, errorEnvelope(requestID, "agent credential is unavailable", false))
		return 2
	}

	callCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	envelope, err := execute(callCtx, spec, baseURL, token, requestID, cfg.HTTPClient)
	if err != nil {
		writeEnvelope(output, errorEnvelope(requestID, "agent API request failed", true))
		return 1
	}
	writeEnvelope(output, envelope)
	if envelope.Status == StatusError {
		return 1
	}
	return 0
}

func parseCommand(args []string) (requestSpec, error) {
	if len(args) < 2 {
		return requestSpec{}, errors.New("unsupported command")
	}
	switch args[0] + " " + args[1] {
	case "capability status":
		flags := flag.NewFlagSet("capability status", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" {
			return requestSpec{}, errors.New("capability status requires --output json")
		}
		return requestSpec{method: http.MethodGet, path: "/api/agent/v1/capabilities/status"}, nil
	case "cluster status":
		flags := flag.NewFlagSet("cluster status", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" {
			return requestSpec{}, errors.New("cluster status requires --output json")
		}
		return requestSpec{method: http.MethodGet, path: "/api/agent/v1/cluster/status"}, nil
	case "workload get":
		flags := flag.NewFlagSet("workload get", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		namespace := flags.String("namespace", "", "")
		kind := flags.String("kind", "", "")
		name := flags.String("name", "", "")
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" || !validName(*namespace) || !validName(*name) || !validKind(*kind) {
			return requestSpec{}, errors.New("workload get requires a valid --namespace, --kind, --name, and --output json")
		}
		return requestSpec{method: http.MethodGet, path: "/api/agent/v1/workloads/get", query: url.Values{"namespace": {*namespace}, "kind": {*kind}, "name": {*name}}}, nil
	case "workload logs":
		flags := flag.NewFlagSet("workload logs", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		namespace := flags.String("namespace", "", "")
		pod := flags.String("pod", "", "")
		container := flags.String("container", "", "")
		tail := flags.Int("tail", 0, "")
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" || !validName(*namespace) || !validName(*pod) || !validContainer(*container) || *tail < 1 || *tail > 200 {
			return requestSpec{}, errors.New("workload logs requires valid --namespace, --pod, --container, --tail, and --output json")
		}
		return requestSpec{method: http.MethodGet, path: "/api/agent/v1/workloads/logs", query: url.Values{"namespace": {*namespace}, "pod": {*pod}, "container": {*container}, "tail": {strconv.Itoa(*tail)}}}, nil
	case "deployment scale":
		flags := flag.NewFlagSet("deployment scale", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		namespace := flags.String("namespace", "", "")
		name := flags.String("name", "", "")
		replicas := flags.Int("replicas", -1, "")
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" || !validName(*namespace) || !validName(*name) || *replicas < 0 || *replicas > 50 {
			return requestSpec{}, errors.New("deployment scale requires valid --namespace, --name, --replicas, and --output json")
		}
		return requestSpec{method: http.MethodPost, path: "/api/agent/v1/deployments/scale", mutation: true, body: map[string]any{"namespace": *namespace, "name": *name, "replicas": *replicas}}, nil
	case "approval get":
		flags := flag.NewFlagSet("approval get", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		id := flags.String("id", "", "")
		output := flags.String("output", "", "")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *output != "json" || !validApprovalID(*id) {
			return requestSpec{}, errors.New("approval get requires a valid --id and --output json")
		}
		return requestSpec{method: http.MethodGet, path: "/api/agent/v1/approvals/" + *id}, nil
	default:
		return requestSpec{}, errors.New("unsupported command")
	}
}

func execute(ctx context.Context, spec requestSpec, baseURL *url.URL, token, requestID string, client *http.Client) (Envelope, error) {
	target := baseURL.ResolveReference(&url.URL{Path: spec.path})
	target.RawQuery = spec.query.Encode()
	var body io.Reader
	if spec.body != nil {
		encoded, err := json.Marshal(spec.body)
		if err != nil {
			return Envelope{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, spec.method, target.String(), body)
	if err != nil {
		return Envelope{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	if spec.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if spec.mutation {
		request.Header.Set("Idempotency-Key", requestID)
	}
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		return Envelope{}, err
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Envelope{}, err
	}
	if len(encoded) > maxResponseBytes {
		return Envelope{}, errors.New("response limit exceeded")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return managerHTTPError(requestID, response.StatusCode, encoded), nil
	}
	var payload agentResponse
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return Envelope{}, err
	}
	if payload.Status == "" {
		payload.Status = StatusOK
	}
	if payload.Summary == "" {
		payload.Summary = "request completed"
	}
	return Envelope{Status: payload.Status, RequestID: requestID, OperationID: payload.OperationID, Data: payload.Data, Summary: payload.Summary, Retryable: payload.Retryable}, nil
}

func managerHTTPError(requestID string, statusCode int, encoded []byte) Envelope {
	var payload agentResponse
	if err := json.Unmarshal(encoded, &payload); err == nil && strings.TrimSpace(payload.Summary) != "" {
		return Envelope{
			Status:      StatusError,
			RequestID:   requestID,
			OperationID: payload.OperationID,
			Data:        payload.Data,
			Summary:     payload.Summary,
			Retryable:   payload.Retryable,
		}
	}
	return errorEnvelope(requestID, fmt.Sprintf("agent API rejected the request (HTTP %d)", statusCode), statusCode >= 500)
}

func parseBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid base URL")
	}
	return parsed, nil
}

func readToken(path string) (string, error) {
	if path == "" {
		return "", errors.New("missing token file")
	}
	encoded, err := os.ReadFile(path)
	if err != nil || len(encoded) == 0 || len(encoded) > maxTokenBytes {
		return "", errors.New("invalid token file")
	}
	token := strings.TrimSpace(string(encoded))
	if token == "" || len(token) > maxTokenBytes || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("invalid token")
	}
	return token, nil
}

func validName(value string) bool {
	return resourceNamePattern.MatchString(value)
}

func validContainer(value string) bool {
	return len(value) <= 63 && regexp.MustCompile(`^[A-Za-z0-9](?:[-_A-Za-z0-9.]{0,61}[A-Za-z0-9])?$`).MatchString(value)
}

func validKind(value string) bool {
	return value == "deployment" || value == "statefulset" || value == "daemonset"
}

func validApprovalID(value string) bool {
	return len(value) > 0 && len(value) <= 128 && regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(value)
}

func errorEnvelope(requestID, summary string, retryable bool) Envelope {
	return Envelope{Status: StatusError, RequestID: requestID, Summary: summary, Retryable: retryable}
}

func writeEnvelope(output io.Writer, envelope Envelope) {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		_, _ = fmt.Fprintln(output, `{"status":"error","summary":"unable to encode response","retryable":false}`)
		return
	}
	_, _ = output.Write(append(encoded, '\n'))
}
