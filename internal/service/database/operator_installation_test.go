package database

import (
	"context"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestOperatorCleanupReportsCurrentAttempt(t *testing.T) {
	name := "helm-delete-" + mysqlOperatorHelmName
	now := metav1.Now()
	for _, tc := range []struct {
		name, want string
		objects    []runtime.Object
		failure    bool
	}{
		{name: "waiting_controller", want: "等待 Helm 控制器"},
		{name: "failed_job", want: "BackoffLimitExceeded", failure: true, objects: []runtime.Object{&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kube-system"}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"}}}}}},
		{name: "image_pull", want: "registry timeout", objects: []runtime.Object{
			&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kube-system", UID: "current"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-old", Namespace: "kube-system", Labels: map[string]string{"job-name": name}, DeletionTimestamp: &now}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "obsolete"}}}}}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-new", Namespace: "kube-system", UID: "pod-new", Labels: map[string]string{"job-name": name}, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: "current"}}}, Status: corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "registry timeout"}}}}}},
		}},
		{name: "pull_event", want: "Pulling image", objects: []runtime.Object{
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-new", Namespace: "kube-system", UID: "pod-new", Labels: map[string]string{"job-name": name}}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
			&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "pull", Namespace: "kube-system"}, InvolvedObject: corev1.ObjectReference{UID: "pod-new"}, Reason: "Pulling", Message: "Pulling image klipper-helm"},
		}},
		{name: "succeeded_waits_cleanup", want: "等待清理 HelmChart", objects: []runtime.Object{&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kube-system"}, Status: batchv1.JobStatus{Succeeded: 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(tc.objects...)}
			result, err := operatorCleanupStatus(context.Background(), client, &OperatorInstallation{State: "cancelling"})
			if err != nil || !strings.Contains(result.Progress, tc.want) || (result.Failure != "") != tc.failure || result.State != "cancelling" {
				t.Fatalf("cleanup=%+v err=%v", result, err)
			}
		})
	}
}
