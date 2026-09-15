package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAgentRegistryDiagnosticsAreScopedAndNeverExposeCredentials(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	mirror := &model.NodeRegistryMirror{Name: "registry-k8s", Registry: "registry.k8s.io", Endpoints: `["https://user:secret@mirror.example.com"]`, VerificationImage: "registry.k8s.io/pause:3.10", Credential: "encrypted-credential", Enabled: true, LastVerifyStatus: "succeeded"}
	if err := s.CreateNodeRegistryMirror(mirror); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	server := &model.Server{Name: "node-1", Host: "10.0.0.1", SSHUser: "root", SSHAuthType: "key", K8sNodeName: "node-1"}
	if err := s.CreateServer(server); err != nil {
		t.Fatalf("create server: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "kube-system"}, Spec: corev1.PodSpec{NodeName: "node-1"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Image: "registry.k8s.io/pause:3.10", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "token=should-not-leak"}}}}}})}
	handler := newTestAgentHandler(s, client, agentAuthenticatorStub{instance: instance}).WithRegistryVerifier(registryservice.NodeVerifierFunc(func(_ context.Context, _ *model.Server, endpoints []string) ([]registryservice.EndpointResult, error) {
		return []registryservice.EndpointResult{{Endpoint: endpoints[0], DNS: "ok", HTTP: "200"}}, nil
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/registries/status", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.RegistryStatus(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected registry deny, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryRead, Namespace: "*", Enabled: true}, {RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryVerify, Namespace: "*", Enabled: true}, {RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadRead, Namespace: "kube-system", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.RegistryStatus(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "encrypted-credential") {
		t.Fatalf("unsafe registry status: %d %s", recorder.Code, recorder.Body.String())
	}
	diagnose := httptest.NewRequest(http.MethodGet, "/api/agent/v1/images/diagnose?namespace=kube-system&pod=pending-pod", nil)
	diagnose.Header.Set("Authorization", "Bearer agent-token")
	recorder = httptest.NewRecorder()
	handler.ImageDiagnose(recorder, diagnose)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "image_pull_failure_detected") || strings.Contains(recorder.Body.String(), "should-not-leak") {
		t.Fatalf("unsafe diagnosis: %d %s", recorder.Code, recorder.Body.String())
	}
	verify := httptest.NewRequest(http.MethodGet, "/api/agent/v1/registries/node-verify?node=node-1&registry=registry.k8s.io", nil)
	verify.Header.Set("Authorization", "Bearer agent-token")
	recorder = httptest.NewRecorder()
	handler.RegistryNodeVerify(recorder, verify)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"dns":"ok"`) {
		t.Fatalf("verify: %d %s", recorder.Code, recorder.Body.String())
	}

	failingHandler := newTestAgentHandler(s, client, agentAuthenticatorStub{instance: instance}).WithRegistryVerifier(registryservice.NodeVerifierFunc(func(context.Context, *model.Server, []string) ([]registryservice.EndpointResult, error) {
		return nil, fmt.Errorf("node verification returned no endpoint results: token=should-not-leak")
	}))
	recorder = httptest.NewRecorder()
	failingHandler.RegistryNodeVerify(recorder, verify)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "no endpoint results") || strings.Contains(recorder.Body.String(), "should-not-leak") {
		t.Fatalf("unsafe verification failure: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRegistryPullCheckRequiresApprovalAndUsesConfiguredImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.CreateNodeRegistryMirror(&model.NodeRegistryMirror{Name: "registry-k8s", Registry: "registry.k8s.io", Endpoints: `["https://mirror.example.com"]`, VerificationImage: "registry.k8s.io/pause:3.10", Enabled: true}); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	if err := s.CreateServer(&model.Server{Name: "node-1", Host: "10.0.0.1", SSHUser: "root", SSHAuthType: "key", K8sNodeName: "node-1"}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryPullCheck, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	agentHandler := newTestAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/registries/node-pull-check", bytes.NewBufferString(`{"node":"node-1","registry":"registry.k8s.io"}`))
	request.Header.Set("Authorization", "Bearer agent-token")
	request.Header.Set("X-Request-ID", "request_456")
	request.Header.Set("Idempotency-Key", "request_456")
	recorder := httptest.NewRecorder()
	agentHandler.RegistryNodePullCheck(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("pull check request: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.OperationID == "" {
		t.Fatalf("decode operation: %v %s", err, recorder.Body.String())
	}
	calledImage := ""
	approvalHandler := newTestAgentOperationHandler(s, nil).WithRegistryPullExecutor(registryservice.PullExecutorFunc(func(_ context.Context, _ *model.Server, image string) error { calledImage = image; return nil }))
	router := gin.New()
	router.POST("/agent-operations/:operationID/approve", func(c *gin.Context) { c.Set("user_id", uint(7)); approvalHandler.Approve(c) })
	approval := serve(router, newJSONRequest(http.MethodPost, "/agent-operations/"+response.OperationID+"/approve", nil))
	if approval.Code != http.StatusOK || calledImage != "registry.k8s.io/pause:3.10" {
		t.Fatalf("pull approval: %d %s image=%q", approval.Code, approval.Body.String(), calledImage)
	}
	events, total, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Limit: 20})
	if err != nil || total != 3 {
		t.Fatalf("expected requested, approved and succeeded events: %#v total=%d err=%v", events, total, err)
	}
	stages := map[string]bool{}
	for _, event := range events {
		if event.OperationID != response.OperationID {
			t.Fatalf("event %q missing lifecycle operation ID: %#v", event.Action, event)
		}
		if event.Action == "agent.registry_pull_check_requested" && event.RequestID != "request_456" {
			t.Fatalf("requested event missing request correlation: %#v", event)
		}
		stages[event.Action] = true
	}
	for _, action := range []string{"agent.registry_pull_check_requested", "agent.operation_approved", "agent.operation_succeeded"} {
		if !stages[action] {
			t.Fatalf("missing lifecycle stage %q: %#v", action, stages)
		}
	}
}

func TestAgentRegistryPullCheckPersistsSanitizedExecutionFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.CreateNodeRegistryMirror(&model.NodeRegistryMirror{Name: "registry-k8s", Registry: "registry.k8s.io", Endpoints: `["https://mirror.example.com"]`, VerificationImage: "registry.k8s.io/pause:3.10", Enabled: true}); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	if err := s.CreateServer(&model.Server{Name: "node-1", Host: "10.0.0.1", SSHUser: "root", SSHAuthType: "key", K8sNodeName: "node-1"}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryPullCheck, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	agentHandler := newTestAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/registries/node-pull-check", bytes.NewBufferString(`{"node":"node-1","registry":"registry.k8s.io"}`))
	request.Header.Set("Authorization", "Bearer agent-token")
	request.Header.Set("X-Request-ID", "request_457")
	request.Header.Set("Idempotency-Key", "request_457")
	recorder := httptest.NewRecorder()
	agentHandler.RegistryNodePullCheck(recorder, request)
	var response struct {
		OperationID string `json:"operation_id"`
	}
	if recorder.Code != http.StatusAccepted || json.Unmarshal(recorder.Body.Bytes(), &response) != nil {
		t.Fatalf("pull check request: %d %s", recorder.Code, recorder.Body.String())
	}
	approvalHandler := newTestAgentOperationHandler(s, nil).WithRegistryPullExecutor(registryservice.PullExecutorFunc(func(_ context.Context, _ *model.Server, _ string) error {
		return fmt.Errorf("rpc error: code = Unknown desc = pull timed out token=not-for-history")
	}))
	router := gin.New()
	router.POST("/agent-operations/:operationID/approve", func(c *gin.Context) { c.Set("user_id", uint(7)); approvalHandler.Approve(c) })
	approval := serve(router, newJSONRequest(http.MethodPost, "/agent-operations/"+response.OperationID+"/approve", nil))
	if approval.Code != http.StatusBadGateway {
		t.Fatalf("approve: %d %s", approval.Code, approval.Body.String())
	}
	events, total, err := s.ListAuditLogsFiltered(model.AuditLogFilter{Action: "agent.operation_failed", Outcome: model.AuditOutcomeFailed, Limit: 20})
	if err != nil || total != 1 || len(events) != 1 || events[0].OperationID != response.OperationID {
		t.Fatalf("expected failed lifecycle audit: %#v total=%d err=%v", events, total, err)
	}
	operation, err := s.GetAgentOperation(response.OperationID)
	if err != nil || operation.Status != model.AgentOperationFailed || !strings.Contains(operation.ErrorSummary, "pull timed out") || strings.Contains(operation.ErrorSummary, "not-for-history") {
		t.Fatalf("unexpected failed operation: %+v err=%v", operation, err)
	}
	listHandler := newTestAgentOperationHandler(s, nil)
	listRouter := gin.New()
	listRouter.GET("/runtimes/:id/agent-operations", listHandler.ListOperations)
	list := serve(listRouter, newJSONRequest(http.MethodGet, "/runtimes/"+itoa(instance.ID)+"/agent-operations", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "pull timed out") || strings.Contains(list.Body.String(), "not-for-history") || strings.Contains(list.Body.String(), `"parameters"`) {
		t.Fatalf("unexpected operation history: %d %s", list.Code, list.Body.String())
	}
}

func TestAgentDNSAndRegistryProxyDiagnosticsAreClusterScopedAndRedacted(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.SaveRegistryProxy(&model.RegistryProxy{Name: "Docker Hub", Registry: "docker.io", UpstreamURL: "https://registry-1.docker.io", NodeName: "node-1", EndpointHost: "10.0.0.5", NodePort: 30500, Status: "ready", LastDiagnosticStatus: "upstream_connect_timeout", LastDiagnosticError: "token=secret upstream connection timed out"}); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: coreDNSConfigMap, Namespace: coreDNSNamespace}, Data: map[string]string{"Corefile": ".:53 {\n  forward . 1.1.1.1\n}\n"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "coredns-a", Namespace: coreDNSNamespace, Labels: map[string]string{"k8s-app": "kube-dns"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	handler := newTestAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
	for _, test := range []struct {
		path    string
		handler http.HandlerFunc
	}{{"/api/agent/v1/dns/status", handler.DNSStatus}, {"/api/agent/v1/dns/resolve?name=registry-1.docker.io", handler.DNSResolve}, {"/api/agent/v1/registries/proxy-diagnose?registry=docker.io", handler.RegistryProxyDiagnose}} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		req.Header.Set("Authorization", "Bearer agent-token")
		rec := httptest.NewRecorder()
		test.handler(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected denied %s, got %d", test.path, rec.Code)
		}
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityDNSRead, Namespace: "*", Enabled: true}, {RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryProxyDiagnose, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	for _, test := range []struct {
		path    string
		handler http.HandlerFunc
		expect  string
	}{{"/api/agent/v1/dns/status", handler.DNSStatus, "1.1.1.1"}, {"/api/agent/v1/dns/resolve?name=registry-1.docker.io", handler.DNSResolve, "upstream_connect_timeout"}, {"/api/agent/v1/registries/proxy-diagnose?registry=docker.io", handler.RegistryProxyDiagnose, "upstream_connect_timeout"}} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		req.Header.Set("Authorization", "Bearer agent-token")
		rec := httptest.NewRecorder()
		test.handler(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), test.expect) || strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("unexpected diagnostic %s: %d %s", test.path, rec.Code, rec.Body.String())
		}
	}
}
