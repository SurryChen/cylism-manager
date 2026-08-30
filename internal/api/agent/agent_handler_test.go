package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type agentAuthenticatorStub struct {
	instance *model.RuntimeInstance
	err      error
}

func TestAgentAlertEndpointsSeparateAlertAndAutomationState(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityAlertRead, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	event, err := s.UpsertAlertEvent(&model.AlertEvent{Fingerprint: "alert-1", AlertName: "NodeDiskHigh", Severity: "warning", NodeName: "node-a", Labels: `{}`, Annotations: `{}`, Status: model.AlertEventFiring, StartsAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create alert event: %v", err)
	}
	event.Status = model.AlertEventAnalyzing
	if err := s.UpdateAlertEvent(event); err != nil {
		t.Fatalf("mark alert analyzing: %v", err)
	}
	handler := NewAgentHandler(s, nil, agentAuthenticatorStub{instance: instance})

	getRequest := httptest.NewRequest(http.MethodGet, "/api/agent/v1/alerts/get?id="+itoa(event.ID), nil)
	getRequest.Header.Set("Authorization", "Bearer agent-token")
	getResponse := httptest.NewRecorder()
	handler.AlertGet(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"automation_status":"analyzing"`) || !strings.Contains(getResponse.Body.String(), `"alert_state":"firing"`) {
		t.Fatalf("expected distinct alert states, got %d: %s", getResponse.Code, getResponse.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/agent/v1/alerts/list", nil)
	listRequest.Header.Set("Authorization", "Bearer agent-token")
	listResponse := httptest.NewRecorder()
	handler.AlertList(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"automation_status":"analyzing"`) || !strings.Contains(listResponse.Body.String(), `"alert_state":"firing"`) {
		t.Fatalf("expected distinct list states, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
}

func TestAgentHandlerWorkloadGetRespectsNamespaceScope(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "operations"}})}
	handler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/workloads/get?namespace=operations&kind=deployment&name=api", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.WorkloadGet(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected out-of-scope deny, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadRead, Namespace: "operations", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.WorkloadGet(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"api"`) {
		t.Fatalf("expected scoped workload, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentHandlerWorkloadLogsSupportsPreviousContainer(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadLogs, Namespace: "operations", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	for _, test := range []struct {
		name     string
		query    string
		previous bool
	}{
		{name: "current container", query: "namespace=operations&pod=api-1&container=api&tail=200", previous: false},
		{name: "previous container", query: "namespace=operations&pod=api-1&container=api&tail=200&previous=true", previous: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			handler := NewAgentHandler(s, &k8s.Client{Clientset: clientset}, agentAuthenticatorStub{instance: instance})
			request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/workloads/logs?"+test.query, nil)
			request.Header.Set("Authorization", "Bearer agent-token")
			recorder := httptest.NewRecorder()
			handler.WorkloadLogs(recorder, request)

			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"previous":`+strconv.FormatBool(test.previous)) {
				t.Fatalf("unexpected logs response: %d %s", recorder.Code, recorder.Body.String())
			}
			for _, action := range clientset.Actions() {
				if action.GetVerb() != "get" || action.GetResource().Resource != "pods" || action.GetSubresource() != "log" {
					continue
				}
				options, ok := action.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions)
				if !ok || options.Previous != test.previous || options.TailLines == nil || *options.TailLines != 200 {
					t.Fatalf("unexpected log options: %#v", options)
				}
				return
			}
			t.Fatal("expected a pod log request")
		})
	}
}

func TestAgentHandlerWorkloadLogsRejectsInvalidPrevious(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	clientset := fake.NewSimpleClientset()
	handler := NewAgentHandler(s, &k8s.Client{Clientset: clientset}, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/workloads/logs?namespace=operations&pod=api-1&container=api&tail=200&previous=invalid", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.WorkloadLogs(recorder, request)

	if recorder.Code != http.StatusBadRequest || len(clientset.Actions()) != 0 {
		t.Fatalf("expected invalid previous rejection without Kubernetes request, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentScaleDoesNotMutateBeforeApprovalAndExecutesAfterApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	initial := int32(1)
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "operations"}, Spec: appsv1.DeploymentSpec{Replicas: &initial}})}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, Namespace: "operations", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	agentHandler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodPost, "/api/agent/v1/deployments/scale", bytes.NewBufferString(`{"namespace":"operations","name":"api","replicas":3}`))
	request.Header.Set("Authorization", "Bearer agent-token")
	request.Header.Set("X-Request-ID", "request_123")
	request.Header.Set("Idempotency-Key", "request_123")
	recorder := httptest.NewRecorder()
	agentHandler.DeploymentScale(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("scale request: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.OperationID == "" {
		t.Fatalf("decode operation: %+v err=%v", response, err)
	}
	deployment, err := client.Clientset.AppsV1().Deployments("operations").Get(t.Context(), "api", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("deployment mutated before approval: %+v err=%v", deployment, err)
	}

	router := gin.New()
	approvalHandler := NewAgentOperationHandler(s, client)
	router.POST("/agent-operations/:operationID/approve", func(c *gin.Context) { c.Set("user_id", uint(7)); approvalHandler.Approve(c) })
	approval := serve(router, newJSONRequest(http.MethodPost, "/agent-operations/"+response.OperationID+"/approve", nil))
	if approval.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", approval.Code, approval.Body.String())
	}
	deployment, err = client.Clientset.AppsV1().Deployments("operations").Get(t.Context(), "api", metav1.GetOptions{})
	if err != nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 3 {
		t.Fatalf("deployment not scaled after approval: %+v err=%v", deployment, err)
	}
	operation, err := s.GetAgentOperation(response.OperationID)
	if err != nil || operation.Status != model.AgentOperationSucceeded {
		t.Fatalf("unexpected operation state: %+v err=%v", operation, err)
	}
}

func TestAgentOperationListFiltersAndDoesNotExposeExecutionParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	for _, operation := range []*model.AgentOperation{
		{OperationID: "op_pending", RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_pending", ChatSessionID: "chat-a", Parameters: `{"token":"must-not-leak"}`, ParametersHash: "hash", Status: model.AgentOperationPendingApproval, Summary: "scale api", ExpiresAt: time.Now().Add(time.Hour)},
		{OperationID: "op_done", RuntimeID: instance.ID, Capability: model.AgentCapabilityDeploymentScale, RequestID: "request_done", ChatSessionID: "chat-b", Parameters: `{"password":"must-not-leak"}`, ParametersHash: "hash", Status: model.AgentOperationSucceeded, Summary: "scale worker", ExpiresAt: time.Now().Add(time.Hour)},
	} {
		if _, _, err := s.CreateAgentOperation(operation); err != nil {
			t.Fatalf("create operation: %v", err)
		}
	}
	handler := NewAgentOperationHandler(s, nil)
	router := gin.New()
	router.GET("/runtimes/:id/agent-operations", handler.ListOperations)
	response := serve(router, newJSONRequest(http.MethodGet, "/runtimes/"+itoa(instance.ID)+"/agent-operations?status=pending_approval&session_id=chat-a", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "op_pending") || strings.Contains(body, "op_done") || strings.Contains(body, "must-not-leak") || strings.Contains(body, `"parameters"`) {
		t.Fatalf("unexpected operation summary: %s", body)
	}
}

func TestRedactAgentTextRemovesAssignmentAndAuthorizationCredentials(t *testing.T) {
	value := redactAgentText("token=abc123 Authorization: Bearer secret-value password: hidden")
	if strings.Contains(value, "abc123") || strings.Contains(value, "secret-value") || strings.Contains(value, "hidden") || !strings.Contains(value, "[REDACTED]") {
		t.Fatalf("agent log redaction leaked a credential: %q", value)
	}
}

func TestAgentOperationErrorSummaryRedactsAndBoundsExecutorDetails(t *testing.T) {
	detail := agentOperationErrorSummary("节点验证镜像拉取失败", fmt.Errorf("rpc failed: token=abc123 Authorization: Bearer secret-value password: hidden %s", strings.Repeat("x", 600)))
	if strings.Contains(detail, "abc123") || strings.Contains(detail, "secret-value") || strings.Contains(detail, "hidden") {
		t.Fatalf("operation error summary leaked a credential: %q", detail)
	}
	if !strings.Contains(detail, "节点验证镜像拉取失败") || !strings.Contains(detail, "[REDACTED]") || len(detail) > agentOperationErrorSummaryLimit {
		t.Fatalf("unexpected operation error summary: len=%d value=%q", len(detail), detail)
	}
}

func TestMaintenanceCleanupFailureSummaryKeepsRedactedExecutorOutput(t *testing.T) {
	detail := maintenanceCleanupFailureSummary("load config file: token=secret-value permission denied", fmt.Errorf("exit status 1"))
	if !strings.Contains(detail, "load config file") || !strings.Contains(detail, "permission denied") || strings.Contains(detail, "secret-value") || !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("unexpected cleanup failure detail: %q", detail)
	}
}

func TestAgentRegistryCommandsUsePaddedBase64ForShellDecoder(t *testing.T) {
	image := "registry.k8s.io/pause:3.10"
	encodedImage := base64.StdEncoding.EncodeToString([]byte(image))
	pullCommand := agentRegistryPullCommand(image)
	if !strings.Contains(encodedImage, "=") || !strings.Contains(pullCommand, "printf %s "+encodedImage+" | base64 -d") {
		t.Fatalf("pull command must use padded standard base64: %s", pullCommand)
	}
	for _, expected := range []string{"sudo -n /usr/local/bin/crictl pull \"$image\"", "sudo -n /var/lib/rancher/k3s/bin/crictl pull \"$image\"", "sudo -n /usr/local/bin/k3s crictl pull \"$image\"", "未找到 crictl 或 k3s 命令"} {
		if !strings.Contains(pullCommand, expected) {
			t.Fatalf("pull command must use an explicit runtime CLI path: %s", pullCommand)
		}
	}
	decodedImage, err := base64.StdEncoding.DecodeString(encodedImage)
	if err != nil || string(decodedImage) != image {
		t.Fatalf("standard base64 image decode failed: %q err=%v", decodedImage, err)
	}

	endpoints := []string{"https://a"}
	encodedEndpoints := base64.StdEncoding.EncodeToString([]byte(`["https://a"]`))
	verificationCommand := agentRegistryVerificationCommand(endpoints)
	if !strings.Contains(encodedEndpoints, "=") || !strings.Contains(verificationCommand, "CYLISM_ENDPOINTS_B64="+encodedEndpoints+" sh -c") {
		t.Fatalf("verification command must use padded standard base64: %s", verificationCommand)
	}
	if !strings.Contains(verificationCommand, "read -r endpoint || [ -n \"$endpoint\" ]") {
		t.Fatalf("verification command must probe a final endpoint without a trailing newline: %s", verificationCommand)
	}
}

func TestParseAgentRegistryEndpointResultsRejectsEmptyVerificationOutput(t *testing.T) {
	results, err := parseAgentRegistryEndpointResults("Warning: Permanently added '10.0.0.1' (ED25519) to the list of known hosts.\nhttps://mirror.example.com|ok|200\n", 1)
	if err != nil || len(results) != 1 || results[0] != (agentRegistryEndpointResult{Endpoint: "https://mirror.example.com", DNS: "ok", HTTP: "200"}) {
		t.Fatalf("parsed results = %#v, %v", results, err)
	}

	_, err = parseAgentRegistryEndpointResults("Warning: remote command emitted no probe rows", 1)
	if err == nil || !strings.Contains(err.Error(), "no endpoint results") {
		t.Fatalf("expected no-result verification error, got %v", err)
	}
}

func (stub agentAuthenticatorStub) AuthenticateAgent(_ context.Context, _ string) (*model.RuntimeInstance, error) {
	return stub.instance, stub.err
}

func TestAgentHandlerClusterStatusRequiresCapabilityGrant(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{ID: 0, Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})}
	handler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance})

	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/cluster/status", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.ClusterStatus(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected deny without grant, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{{RuntimeID: instance.ID, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true}}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.ClusterStatus(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"ok"`) || !strings.Contains(recorder.Body.String(), `"node_count":1`) {
		t.Fatalf("expected granted response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentHandlerCapabilityStatusReturnsEffectiveScopes(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadRead, Namespace: "operations", Enabled: true},
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	handler := NewAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/capabilities/status", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.CapabilityStatus(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"scope":"cluster"`) || !strings.Contains(recorder.Body.String(), `"operations"`) {
		t.Fatalf("unexpected capability status: %d %s", recorder.Code, recorder.Body.String())
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
	proxy := &model.RegistryProxy{Name: "Docker Hub", Registry: "docker.io", UpstreamURL: "https://registry-1.docker.io", NodeName: "node-1", EndpointHost: "10.0.0.5", NodePort: 30500, CacheLimitGi: 5, CleanupIntervalHours: 24, Status: "ready", LastDiagnosticStatus: "upstream_connect_timeout", LastDiagnosticError: "token=secret upstream connection timed out"}
	if err := s.SaveRegistryProxy(proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: coreDNSConfigMap, Namespace: coreDNSNamespace}, Data: map[string]string{"Corefile": ".:53 {\n  forward . 1.1.1.1\n}\n"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "coredns-a", Namespace: coreDNSNamespace, Labels: map[string]string{"k8s-app": "kube-dns"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	handler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
	for _, test := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/api/agent/v1/dns/status", handler.DNSStatus},
		{"/api/agent/v1/dns/resolve?name=registry-1.docker.io", handler.DNSResolve},
		{"/api/agent/v1/registries/proxy-diagnose?registry=docker.io", handler.RegistryProxyDiagnose},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer agent-token")
		recorder := httptest.NewRecorder()
		test.handler(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected denied %s, got %d: %s", test.path, recorder.Code, recorder.Body.String())
		}
	}
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityDNSRead, Namespace: "*", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityRegistryProxyDiagnose, Namespace: "*", Enabled: true},
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	for _, test := range []struct {
		path    string
		handler http.HandlerFunc
		expect  string
	}{
		{"/api/agent/v1/dns/status", handler.DNSStatus, "1.1.1.1"},
		{"/api/agent/v1/dns/resolve?name=registry-1.docker.io", handler.DNSResolve, "upstream_connect_timeout"},
		{"/api/agent/v1/registries/proxy-diagnose?registry=docker.io", handler.RegistryProxyDiagnose, "upstream_connect_timeout"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer agent-token")
		recorder := httptest.NewRecorder()
		test.handler(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.expect) || strings.Contains(recorder.Body.String(), "secret") {
			t.Fatalf("unexpected diagnostic %s: %d %s", test.path, recorder.Code, recorder.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/agent/v1/dns/resolve?name=example.com", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	recorder := httptest.NewRecorder()
	handler.DNSResolve(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("allowlist bypass must fail: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentHandlerProvidesScopedPendingPodDiagnostics(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	instance := &model.RuntimeInstance{Name: "nanobot-main", RuntimeType: model.RuntimeTypeNanobot, DeploymentMode: model.RuntimeDeploymentManaged, Namespace: "cylism-assistant", Image: "example/nanobot", Status: model.RuntimeStatusReady, AgentToolEnabled: true}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "local-path-provisioner-abc", Namespace: "kube-system"}, Status: corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/1 nodes are available"}}}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "failed-scheduling", Namespace: "kube-system"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "local-path-provisioner-abc"}, Type: corev1.EventTypeWarning, Reason: "FailedScheduling", Message: "0/1 nodes are available: insufficient cpu"},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "kube-system"}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{corev1.LabelHostname: "node-1"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")}}},
	)}
	handler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
	if err := s.ReplaceAgentCapabilityGrants(instance.ID, []model.AgentCapabilityGrant{
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityWorkloadRead, Namespace: "kube-system", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityEventsRead, Namespace: "kube-system", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityStorageRead, Namespace: "kube-system", Enabled: true},
		{RuntimeID: instance.ID, Capability: model.AgentCapabilityClusterRead, Namespace: "*", Enabled: true},
	}); err != nil {
		t.Fatalf("grant diagnostics: %v", err)
	}
	for _, test := range []struct {
		path     string
		handler  http.HandlerFunc
		expected string
	}{
		{"/api/agent/v1/pods/get?namespace=kube-system&name=local-path-provisioner-abc", handler.PodGet, "Pending"},
		{"/api/agent/v1/events/list?namespace=kube-system&involved_kind=pod&involved_name=local-path-provisioner-abc&limit=20", handler.EventList, "FailedScheduling"},
		{"/api/agent/v1/pvcs/get?namespace=kube-system&name=data", handler.PVCGet, "Pending"},
		{"/api/agent/v1/nodes/get?name=node-1", handler.NodeGet, "node-1"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer agent-token")
		test.handler(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.expected) {
			t.Fatalf("diagnostic %s = %d: %s", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

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
	handler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance}).WithRegistryVerifier(func(_ *model.Server, endpoints []string) ([]agentRegistryEndpointResult, error) {
		return []agentRegistryEndpointResult{{Endpoint: endpoints[0], DNS: "ok", HTTP: "200"}}, nil
	})
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

	failingHandler := NewAgentHandler(s, client, agentAuthenticatorStub{instance: instance}).WithRegistryVerifier(func(_ *model.Server, _ []string) ([]agentRegistryEndpointResult, error) {
		return nil, fmt.Errorf("node verification returned no endpoint results: token=should-not-leak")
	})
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
	agentHandler := NewAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
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
	approvalHandler := NewAgentOperationHandler(s, nil).WithRegistryPullExecutor(func(_ *model.Server, image string) error { calledImage = image; return nil })
	router := gin.New()
	router.POST("/agent-operations/:operationID/approve", func(c *gin.Context) { c.Set("user_id", uint(7)); approvalHandler.Approve(c) })
	approval := serve(router, newJSONRequest(http.MethodPost, "/agent-operations/"+response.OperationID+"/approve", nil))
	if approval.Code != http.StatusOK || calledImage != "registry.k8s.io/pause:3.10" {
		t.Fatalf("pull approval: %d %s image=%q", approval.Code, approval.Body.String(), calledImage)
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
	agentHandler := NewAgentHandler(s, &k8s.Client{Clientset: fake.NewSimpleClientset()}, agentAuthenticatorStub{instance: instance})
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
	approvalHandler := NewAgentOperationHandler(s, nil).WithRegistryPullExecutor(func(_ *model.Server, _ string) error {
		return fmt.Errorf("rpc error: code = Unknown desc = pull timed out token=not-for-history")
	})
	router := gin.New()
	router.POST("/agent-operations/:operationID/approve", func(c *gin.Context) { c.Set("user_id", uint(7)); approvalHandler.Approve(c) })
	approval := serve(router, newJSONRequest(http.MethodPost, "/agent-operations/"+response.OperationID+"/approve", nil))
	if approval.Code != http.StatusBadGateway {
		t.Fatalf("approve: %d %s", approval.Code, approval.Body.String())
	}
	operation, err := s.GetAgentOperation(response.OperationID)
	if err != nil || operation.Status != model.AgentOperationFailed || !strings.Contains(operation.ErrorSummary, "pull timed out") || strings.Contains(operation.ErrorSummary, "not-for-history") {
		t.Fatalf("unexpected failed operation: %+v err=%v", operation, err)
	}
	listHandler := NewAgentOperationHandler(s, nil)
	listRouter := gin.New()
	listRouter.GET("/runtimes/:id/agent-operations", listHandler.ListOperations)
	list := serve(listRouter, newJSONRequest(http.MethodGet, "/runtimes/"+itoa(instance.ID)+"/agent-operations", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "pull timed out") || strings.Contains(list.Body.String(), "not-for-history") || strings.Contains(list.Body.String(), `"parameters"`) {
		t.Fatalf("unexpected operation history: %d %s", list.Code, list.Body.String())
	}
}
