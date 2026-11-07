package operations

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestImagePullFailureReportsActionableStatus(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
		Name:  "mysql-shell",
		Image: "registry.example/mysql-shell:missing",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "ImagePullBackOff",
		}},
	}}}}
	err := imagePullFailure(pod)
	var pullErr *imagePullError
	if !errors.As(err, &pullErr) || !strings.Contains(err.Error(), "registry.example/mysql-shell:missing") {
		t.Fatalf("expected actionable image pull error, got %v", err)
	}
	pod.Status.ContainerStatuses[0].State.Waiting.Reason = "ContainerCreating"
	if err := imagePullFailure(pod); err != nil {
		t.Fatalf("transient startup must not be reported as image pull failure: %v", err)
	}
}

func TestDiscoveryErrorIdentifiesStageWithoutRawCredentials(t *testing.T) {
	for _, test := range []struct {
		code string
		want string
	}{
		{"mysql_command_failed@source_connect:1045", "连接源库失败：MySQL 拒绝了账号或权限"},
		{"mysql_command_failed@target_channels:1227", "读取从库复制通道失败（MySQL 错误码 1227）"},
		{"mysql_command_failed@target_observation:0", "读取从库只读和复制状态失败"},
	} {
		message := errorMessage(test.code)
		if !strings.Contains(message, test.want) || strings.Contains(message, "password") {
			t.Fatalf("unsafe or unclear error %q: %q", test.code, message)
		}
	}
}
