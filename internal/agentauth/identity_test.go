package agentauth

import (
	"context"
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type reviewerStub struct {
	identity Identity
	err      error
}

func (stub reviewerStub) Review(_ context.Context, _ string, _ string) (Identity, error) {
	return stub.identity, stub.err
}

type runtimeLookupStub struct {
	runtime *model.RuntimeInstance
	err     error
}

func (stub runtimeLookupStub) GetRuntime(_ uint) (*model.RuntimeInstance, error) {
	return stub.runtime, stub.err
}

func TestAuthorizerAcceptsOnlyMappedRuntimeIdentityWithExpectedAudience(t *testing.T) {
	instance := &model.RuntimeInstance{ID: 7, Name: "nanobot-main", Namespace: runtime.DefaultNamespace, DeploymentMode: model.RuntimeDeploymentManaged, AgentToolEnabled: true}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: runtime.RuntimeAgentServiceAccountName(instance), Namespace: instance.Namespace, UID: "sa-uid", Labels: map[string]string{runtime.RuntimeIDLabel: "7", k8s.ManagedByLabel: k8s.ManagedByValue}}})}
	authorizer := NewRuntimeTokenAuthorizer(client, runtimeLookupStub{runtime: instance}, reviewerStub{identity: Identity{Namespace: instance.Namespace, ServiceAccount: runtime.RuntimeAgentServiceAccountName(instance), UID: "sa-uid", Audience: runtime.RuntimeInstallerTokenAudience}})

	if err := authorizer.AuthorizeInstaller(context.Background(), "token"); err != nil {
		t.Fatalf("expected installer identity to be accepted: %v", err)
	}
	if err := authorizer.AuthorizeAgent(context.Background(), "token"); err == nil {
		t.Fatal("installer token must not authorize Agent API")
	}
}

func TestAuthorizerRejectsUnmappedDisabledOrInvalidIdentity(t *testing.T) {
	instance := &model.RuntimeInstance{ID: 7, Name: "nanobot-main", Namespace: runtime.DefaultNamespace, DeploymentMode: model.RuntimeDeploymentManaged, AgentToolEnabled: false}
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: runtime.RuntimeAgentServiceAccountName(instance), Namespace: instance.Namespace, UID: "sa-uid", Labels: map[string]string{runtime.RuntimeIDLabel: "7", k8s.ManagedByLabel: k8s.ManagedByValue}}})}
	cases := []struct {
		name     string
		identity Identity
		lookup   runtimeLookupStub
	}{
		{name: "disabled runtime", identity: Identity{Namespace: instance.Namespace, ServiceAccount: runtime.RuntimeAgentServiceAccountName(instance), UID: "sa-uid", Audience: runtime.RuntimeInstallerTokenAudience}, lookup: runtimeLookupStub{runtime: instance}},
		{name: "wrong service account", identity: Identity{Namespace: instance.Namespace, ServiceAccount: "other", UID: "sa-uid", Audience: runtime.RuntimeInstallerTokenAudience}, lookup: runtimeLookupStub{runtime: instance}},
		{name: "wrong audience", identity: Identity{Namespace: instance.Namespace, ServiceAccount: runtime.RuntimeAgentServiceAccountName(instance), UID: "sa-uid", Audience: runtime.RuntimeAgentTokenAudience}, lookup: runtimeLookupStub{runtime: &model.RuntimeInstance{ID: 7, Name: "nanobot-main", Namespace: runtime.DefaultNamespace, DeploymentMode: model.RuntimeDeploymentManaged, AgentToolEnabled: true}}},
		{name: "lookup failure", identity: Identity{Namespace: instance.Namespace, ServiceAccount: runtime.RuntimeAgentServiceAccountName(instance), UID: "sa-uid", Audience: runtime.RuntimeInstallerTokenAudience}, lookup: runtimeLookupStub{err: errors.New("not found")}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			authorizer := NewRuntimeTokenAuthorizer(client, testCase.lookup, reviewerStub{identity: testCase.identity})
			if err := authorizer.AuthorizeInstaller(context.Background(), "token"); err == nil {
				t.Fatal("expected identity rejection")
			}
		})
	}
}
