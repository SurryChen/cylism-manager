// Package agentauth authenticates projected Runtime workload identities.
package agentauth

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errUnauthorizedRuntime = errors.New("unauthorized runtime identity")

type Identity struct {
	Namespace      string
	ServiceAccount string
	UID            string
	Audience       string
}

type Reviewer interface {
	Review(ctx context.Context, token, audience string) (Identity, error)
}

type RuntimeLookup interface {
	GetRuntime(id uint) (*model.RuntimeInstance, error)
}

type KubernetesReviewer struct {
	Client *k8s.Client
}

func (r KubernetesReviewer) Review(ctx context.Context, token, audience string) (Identity, error) {
	if r.Client == nil || r.Client.Clientset == nil || token == "" || audience == "" {
		return Identity{}, errUnauthorizedRuntime
	}
	review, err := r.Client.Clientset.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: token, Audiences: []string{audience}}}, metav1.CreateOptions{})
	if err != nil || !review.Status.Authenticated || !containsAudience(review.Status.Audiences, audience) {
		return Identity{}, errUnauthorizedRuntime
	}
	namespace, serviceAccount, ok := parseServiceAccountUsername(review.Status.User.Username)
	if !ok || review.Status.User.UID == "" {
		return Identity{}, errUnauthorizedRuntime
	}
	return Identity{Namespace: namespace, ServiceAccount: serviceAccount, UID: review.Status.User.UID, Audience: audience}, nil
}

type RuntimeTokenAuthorizer struct {
	client   *k8s.Client
	runtimes RuntimeLookup
	reviewer Reviewer
}

func NewRuntimeTokenAuthorizer(client *k8s.Client, runtimes RuntimeLookup, reviewer Reviewer) *RuntimeTokenAuthorizer {
	if reviewer == nil {
		reviewer = KubernetesReviewer{Client: client}
	}
	return &RuntimeTokenAuthorizer{client: client, runtimes: runtimes, reviewer: reviewer}
}

func (a *RuntimeTokenAuthorizer) AuthorizeInstaller(ctx context.Context, token string) error {
	return a.authorize(ctx, token, runtime.RuntimeInstallerTokenAudience)
}

func (a *RuntimeTokenAuthorizer) AuthorizeAgent(ctx context.Context, token string) error {
	_, err := a.AuthenticateAgent(ctx, token)
	return err
}

func (a *RuntimeTokenAuthorizer) authorize(ctx context.Context, token, expectedAudience string) error {
	_, err := a.authenticate(ctx, token, expectedAudience)
	return err
}

// AuthenticateAgent resolves a verified agent-token caller to its Runtime.
func (a *RuntimeTokenAuthorizer) AuthenticateAgent(ctx context.Context, token string) (*model.RuntimeInstance, error) {
	return a.authenticate(ctx, token, runtime.RuntimeAgentTokenAudience)
}

func (a *RuntimeTokenAuthorizer) authenticate(ctx context.Context, token, expectedAudience string) (*model.RuntimeInstance, error) {
	if a == nil || a.client == nil || a.client.Clientset == nil || a.runtimes == nil || a.reviewer == nil {
		return nil, errUnauthorizedRuntime
	}
	identity, err := a.reviewer.Review(ctx, token, expectedAudience)
	if err != nil || identity.Audience != expectedAudience {
		return nil, errUnauthorizedRuntime
	}
	serviceAccount, err := a.client.Clientset.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.ServiceAccount, metav1.GetOptions{})
	if err != nil || string(serviceAccount.UID) != identity.UID || serviceAccount.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return nil, errUnauthorizedRuntime
	}
	runtimeID, err := strconv.ParseUint(serviceAccount.Labels[runtime.RuntimeIDLabel], 10, 64)
	if err != nil || runtimeID == 0 {
		return nil, errUnauthorizedRuntime
	}
	instance, err := a.runtimes.GetRuntime(uint(runtimeID))
	if err != nil || instance == nil || !instance.AgentToolEnabled || instance.DeploymentMode != model.RuntimeDeploymentManaged || instance.Namespace != identity.Namespace || identity.ServiceAccount != runtime.RuntimeAgentServiceAccountName(instance) {
		return nil, errUnauthorizedRuntime
	}
	return instance, nil
}

func parseServiceAccountUsername(username string) (namespace, name string, ok bool) {
	const prefix = "system:serviceaccount:"
	if len(username) <= len(prefix) || username[:len(prefix)] != prefix {
		return "", "", false
	}
	parts := splitServiceAccount(username[len(prefix):])
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func splitServiceAccount(value string) []string {
	for index, character := range value {
		if character == ':' {
			return []string{value[:index], value[index+1:]}
		}
	}
	return nil
}

func containsAudience(audiences []string, expected string) bool {
	for _, audience := range audiences {
		if audience == expected {
			return true
		}
	}
	return false
}

func (identity Identity) String() string {
	return fmt.Sprintf("%s/%s", identity.Namespace, identity.ServiceAccount)
}
