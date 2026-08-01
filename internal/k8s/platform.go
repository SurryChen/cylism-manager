package k8s

import (
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	platformDeploymentNamespace = "default"
	platformDeploymentName      = "cylism-manager"
	platformContainerName       = "platform"
	platformReleaseAnnotation   = "cylism.io/platform-release"
	platformRestartAnnotation   = "cylism.io/platform-restarted-at"
)

// PlatformDeploymentStatus is the only supported self-update target state.
type PlatformDeploymentStatus struct {
	Image           string `json:"image"`
	ReleaseID       uint   `json:"release_id,omitempty"`
	DesiredReplicas int32  `json:"desired_replicas"`
	ReadyReplicas   int32  `json:"ready_replicas"`
	Failure         string `json:"failure,omitempty"`
}

func (c *Client) PlatformDeploymentStatus() (*PlatformDeploymentStatus, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read platform deployment: %w", err)
	}
	status := &PlatformDeploymentStatus{DesiredReplicas: 1, ReadyReplicas: deployment.Status.AvailableReplicas}
	if deployment.Spec.Replicas != nil {
		status.DesiredReplicas = *deployment.Spec.Replicas
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == platformContainerName {
			status.Image = container.Image
			break
		}
	}
	if status.Image == "" {
		return nil, fmt.Errorf("platform deployment does not contain %q container", platformContainerName)
	}
	if raw := deployment.Spec.Template.Annotations[platformReleaseAnnotation]; raw != "" {
		value, _ := strconv.ParseUint(raw, 10, 64)
		status.ReleaseID = uint(value)
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == "True" {
			status.Failure = condition.Message
		}
	}
	return status, nil
}

// UpdatePlatformDeployment changes exactly one known Deployment/container pair.
func (c *Client) UpdatePlatformDeployment(image string, releaseID uint) (string, error) {
	deployment, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(c.Ctx(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read platform deployment: %w", err)
	}
	previous := ""
	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		if container.Name == platformContainerName {
			previous = container.Image
			container.Image = image
			break
		}
	}
	if previous == "" {
		return "", fmt.Errorf("platform deployment does not contain %q container", platformContainerName)
	}
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations[platformReleaseAnnotation] = strconv.FormatUint(uint64(releaseID), 10)
	deployment.Spec.Template.Annotations[platformRestartAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := c.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Update(c.Ctx(), deployment, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("update platform deployment: %w", err)
	}
	return previous, nil
}
