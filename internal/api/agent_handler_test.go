package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type agentAuthenticatorStub struct {
	instance *model.RuntimeInstance
	err      error
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

func TestRedactAgentTextRemovesAssignmentAndAuthorizationCredentials(t *testing.T) {
	value := redactAgentText("token=abc123 Authorization: Bearer secret-value password: hidden")
	if strings.Contains(value, "abc123") || strings.Contains(value, "secret-value") || strings.Contains(value, "hidden") || !strings.Contains(value, "[REDACTED]") {
		t.Fatalf("agent log redaction leaked a credential: %q", value)
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
