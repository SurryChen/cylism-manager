package database

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateRetryPodOnlyFailedOwnedPod(t *testing.T) {
	controller := true
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "cylism-mysql-cluster-1"}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName + "-0", Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": item.ResourceName}, OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: item.ResourceName, Controller: &controller}}},
		Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}},
	}
	if err := ValidateRetryPod(item, pod); err != nil {
		t.Fatal(err)
	}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if err := ValidateRetryPod(item, pod); err == nil || !strings.Contains(err.Error(), "已就绪") {
		t.Fatalf("ready pod must be protected: %v", err)
	}
	pod.Status.Conditions = nil
	pod.Labels["mysql.oracle.com/cluster"] = "other"
	if err := ValidateRetryPod(item, pod); err == nil {
		t.Fatal("another cluster's pod must be protected")
	}
}

func TestPendingPodImagesTargetsBlockedInitContainer(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "fixdatadir", Image: "operator:2.3"}, {Name: "initconf", Image: "operator:2.3"}, {Name: "initmysql", Image: "server:26.7"}}, Containers: []corev1.Container{{Name: "mysql", Image: "server:26.7"}}},
		Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{
			{Name: "fixdatadir", ImageID: "sha256:ready", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
			{Name: "initconf", ImageID: "sha256:ready", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
			{Name: "initmysql", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
		}},
	}
	images := PendingPodImages(pod)
	if len(images) != 1 || images[0] != "server:26.7" {
		t.Fatalf("pending images = %v, want only server:26.7", images)
	}
	pod.Status.InitContainerStatuses = nil
	images = PendingPodImages(pod)
	if len(images) != 1 || images[0] != "operator:2.3" {
		t.Fatalf("first pending init image = %v, want only operator:2.3", images)
	}
}

func TestValidateManagedPodAllowsWaitingButNotReady(t *testing.T) {
	controller := true
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "cluster-1"}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "cluster-1-1", Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": "cluster-1"}, OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "cluster-1", Controller: &controller}}}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}}}}}
	if err := ValidateManagedPod(item, pod); err != nil {
		t.Fatalf("waiting pod should permit image pull: %v", err)
	}
	if err := ValidateRetryPod(item, pod); err == nil {
		t.Fatal("waiting pod must not permit retry")
	}
}
