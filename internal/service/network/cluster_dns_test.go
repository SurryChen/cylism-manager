package network

import (
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

func TestForwardTargetsAndPolicyPayload(t *testing.T) {
	targets := ForwardTargets(".:53 {\n  forward . 1.1.1.1 8.8.8.8\n}")
	if len(targets) != 2 || targets[0] != "1.1.1.1" || targets[1] != "8.8.8.8" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	createdAt := time.Now()
	payload := PolicyPayload(&model.ClusterDNSPolicy{Revision: 3, Resolvers: `["1.1.1.1"]`, CreatedAt: createdAt})
	if payload == nil || payload["revision"] != uint(3) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if got, ok := payload["resolvers"].([]string); !ok || len(got) != 1 || got[0] != "1.1.1.1" {
		t.Fatalf("unexpected resolvers: %#v", payload["resolvers"])
	}
	if payload["created_at"] != createdAt {
		t.Fatalf("unexpected created_at: %#v", payload["created_at"])
	}
}

func TestPolicyPayloadNil(t *testing.T) {
	if payload := PolicyPayload(nil); payload != nil {
		t.Fatalf("nil policy should produce nil payload: %#v", payload)
	}
}

func TestCoreDNSPodReady(t *testing.T) {
	if CoreDNSPodReady(nil) {
		t.Fatal("nil pod must not be ready")
	}
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	if !CoreDNSPodReady(pod) {
		t.Fatal("ready pod was not recognized")
	}
}
