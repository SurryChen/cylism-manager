package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestConfigurePrimaryUsesSelectedNodeAndKeepsLaterFailover(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "database", DatabaseNodes: []string{"a", "b", "c"}, PreferredPrimaryNode: "b"}
	pods := []ClusterPod{
		{Name: "database-0", Component: "mysqld", NodeName: "a", Ready: true},
		{Name: "database-2", Component: "mysqld", NodeName: "b", Ready: true},
		{Name: "router", Component: "mysqlrouter", NodeName: "b", Ready: true},
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "database-2", Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": "database", "component": "mysqld", "mysql.oracle.com/cluster-role": "SECONDARY"}}, Spec: corev1.PodSpec{NodeName: "b"}, Status: corev1.PodStatus{PodIP: "10.42.1.7", Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-root", Namespace: "mysql"}, Data: map[string][]byte{"rootUser": []byte("root"), "rootPassword": []byte("private-password")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-test", Namespace: "mysql-operator", Labels: map[string]string{"app.kubernetes.io/name": "mysql-operator"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	calls := 0
	execute := func(_ context.Context, _ *k8s.Client, operator string, input []byte) error {
		calls++
		var payload map[string]any
		if err := json.Unmarshal(input, &payload); err != nil {
			t.Fatal(err)
		}
		if operator != "mysql-operator-test" || payload["host"] != "10.42.1.7" || payload["endpoint"] != "10.42.1.7:3306" || payload["password"] != "private-password" {
			t.Fatalf("wrong execution target or credentials: operator=%s", operator)
		}
		if payload["require_secondary"] != false {
			t.Fatalf("initial role selection must allow an already primary instance: %v", payload)
		}
		return nil
	}
	if err := configurePreferredPrimary(ctx, client, item, pods, execute); err != nil {
		t.Fatal(err)
	}
	item.PrimaryConfigured = true
	// A later automatic failover must not be undone by a status refresh.
	if err := configurePreferredPrimary(ctx, client, item, nil, execute); err != nil || calls != 1 {
		t.Fatalf("configured preference was reapplied: calls=%d, %v", calls, err)
	}
	item.PrimaryConfigured = false
	err := configurePreferredPrimary(ctx, client, item, pods, func(context.Context, *k8s.Client, string, []byte) error {
		return fmt.Errorf("mysqlsh failure: private-password")
	})
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("unsafe or missing configuration error: %v", err)
	}
	err = configurePreferredPrimary(ctx, client, item, pods, func(context.Context, *k8s.Client, string, []byte) error {
		return ErrPrimaryNotReady
	})
	if !errors.Is(err, ErrPrimaryNotReady) {
		t.Fatalf("member synchronization should remain pending: %v", err)
	}
	item.PreferredPrimaryNode = "missing"
	if err := configurePreferredPrimary(ctx, client, item, pods, execute); err == nil {
		t.Fatal("missing preferred instance accepted")
	}
}

func TestSwitchPrimaryRequiresReadySecondaryInCurrentCluster(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "database", DatabaseNodes: []string{"a", "b", "c"}}
	target := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "database-2", Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": "database", "component": "mysqld", "mysql.oracle.com/cluster-role": "SECONDARY"}}, Spec: corev1.PodSpec{NodeName: "b"}, Status: corev1.PodStatus{PodIP: "10.42.1.7", Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(
		target,
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-root", Namespace: "mysql"}, Data: map[string][]byte{"rootUser": []byte("root"), "rootPassword": []byte("private-password")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-test", Namespace: "mysql-operator", Labels: map[string]string{"app.kubernetes.io/name": "mysql-operator"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	called := 0
	execute := func(_ context.Context, _ *k8s.Client, _ string, input []byte) error {
		called++
		var payload map[string]any
		if err := json.Unmarshal(input, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["require_secondary"] != true || payload["host"] != "10.42.1.7" {
			t.Fatalf("wrong switch request: %v", payload)
		}
		return nil
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); err != nil || called != 1 {
		t.Fatalf("ready secondary was not switched: calls=%d err=%v", called, err)
	}
	target.Labels["mysql.oracle.com/cluster-role"] = "PRIMARY"
	if _, err := client.Clientset.CoreV1().Pods("mysql").Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); !errors.Is(err, ErrPrimaryNotSecondary) || called != 1 {
		t.Fatalf("current primary was accepted: calls=%d err=%v", called, err)
	}
	target.Labels["mysql.oracle.com/cluster"] = "other-cluster"
	if _, err := client.Clientset.CoreV1().Pods("mysql").Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); err == nil || called != 1 {
		t.Fatalf("foreign pod was accepted: calls=%d err=%v", called, err)
	}
}
