package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

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
	handler := newTestAgentHandler(s, client, agentAuthenticatorStub{instance: instance})
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
