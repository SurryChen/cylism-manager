package shared

import (
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

func TestForwardTargetsAndPolicyPayload(t *testing.T) {
	targets := ForwardTargets(".:53 {\n  forward . 1.1.1.1 8.8.8.8\n}")
	if len(targets) != 2 || targets[0] != "1.1.1.1" || targets[1] != "8.8.8.8" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	payload := PolicyPayload(&model.ClusterDNSPolicy{Revision: 3, Resolvers: `["1.1.1.1"]`})
	if payload["revision"] != int64(3) && payload["revision"] != uint(3) && payload["revision"] != 3 {
		t.Fatalf("unexpected policy payload: %#v", payload)
	}
	if payload["resolvers"].([]string)[0] != "1.1.1.1" {
		t.Fatalf("unexpected resolvers: %#v", payload)
	}
}

func TestCoreDNSPodReady(t *testing.T) {
	if CoreDNSPodReady(nil) {
		t.Fatal("nil pod should not be ready")
	}
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	if !CoreDNSPodReady(pod) {
		t.Fatal("ready pod was not recognized")
	}
}
