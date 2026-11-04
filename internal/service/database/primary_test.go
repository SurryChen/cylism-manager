package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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

func TestSwitchPrimaryChecksTargetSafetyAndDefersRolesToMySQL(t *testing.T) {
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
		if payload["host"] != "10.42.1.7" {
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
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); err != nil || called != 2 {
		t.Fatalf("stale PRIMARY label prevented live verification: calls=%d err=%v", called, err)
	}
	target.Status.Conditions = nil
	if _, err := client.Clientset.CoreV1().Pods("mysql").Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); err == nil || called != 2 {
		t.Fatal("unready target reached MySQL command")
	}
	target.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if _, err := client.Clientset.CoreV1().Pods("mysql").Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "different-node", true, execute); err == nil || called != 2 {
		t.Fatal("relocated target reached MySQL command")
	}
	target.Labels["mysql.oracle.com/cluster"] = "other-cluster"
	if _, err := client.Clientset.CoreV1().Pods("mysql").Update(ctx, target, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := switchPrimaryPod(ctx, client, item, target.Name, "b", true, execute); err == nil || called != 2 {
		t.Fatalf("foreign pod was accepted: calls=%d err=%v", called, err)
	}
}

// Execute the actual embedded script with a controlled MySQL Shell module so
// role races and no-op switches are covered without a live database.
func TestPrimaryScriptUsesLiveRolesAndIsIdempotent(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the embedded MySQL Shell script")
	}
	const harness = `import json,sys,types
case=json.loads(sys.argv[1])
state={"role":case["role"],"switches":0}
class Session:
    def run_sql(self, sql):
        return types.SimpleNamespace(fetch_one=lambda: [state["role"]] if state["role"] else None)
class Cluster:
    def status(self):
        return {"defaultReplicaSet":{"topology":{"a":{"status":"ONLINE"},"b":{"status":case["member_state"]}}}}
    def set_primary_instance(self, endpoint):
        state["switches"]+=1
        if case["switch_succeeds"]:
            state["role"]="PRIMARY"
shell=types.SimpleNamespace(connect=lambda options: None,open_session=lambda options: Session())
sys.modules["mysqlsh"]=types.SimpleNamespace(globals=types.SimpleNamespace(shell=shell,dba=types.SimpleNamespace(get_cluster=lambda: Cluster())))
try:
    exec(sys.argv[2])
finally:
    print("TEST_SWITCH_COUNT="+str(state["switches"]))
`
	for _, tc := range []struct {
		name, role, memberState, marker string
		switches                        int
		switchSucceeds                  bool
		exitCode                        int
	}{
		{name: "already primary despite secondary label", role: "PRIMARY", memberState: "ONLINE", marker: "CYLISM_PRIMARY_CONFIGURED", switches: 0, switchSucceeds: true},
		{name: "secondary switched and verified", role: "SECONDARY", memberState: "ONLINE", marker: "CYLISM_PRIMARY_CONFIGURED", switches: 1, switchSucceeds: true},
		{name: "unknown role rejected", role: "UNKNOWN", memberState: "ONLINE", marker: "CYLISM_PRIMARY_ROLE_UNKNOWN", exitCode: 3},
		{name: "missing membership rejected", role: "", memberState: "ONLINE", marker: "CYLISM_PRIMARY_ROLE_UNKNOWN", exitCode: 3},
		{name: "switch result must be verified", role: "SECONDARY", memberState: "ONLINE", marker: "CYLISM_PRIMARY_FAILED", switches: 1, exitCode: 1},
		{name: "offline cluster rejected", role: "SECONDARY", memberState: "RECOVERING", marker: "CYLISM_PRIMARY_WAITING", exitCode: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scenario, _ := json.Marshal(map[string]any{"role": tc.role, "member_state": tc.memberState, "switch_succeeds": tc.switchSucceeds})
			cmd := exec.Command(python, "-c", harness, string(scenario), configurePrimaryScript)
			cmd.Stdin = strings.NewReader(`{"host":"10.42.1.7","endpoint":"10.42.1.7:3306","user":"root","password":"private-test-password","instances":2}`)
			output, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tc.exitCode || !strings.Contains(string(output), tc.marker) || !strings.Contains(string(output), fmt.Sprintf("TEST_SWITCH_COUNT=%d", tc.switches)) {
				t.Fatalf("unexpected script result: code=%d output=%s", code, output)
			}
			if tc.exitCode != 0 && strings.Contains(string(output), "CYLISM_PRIMARY_CONFIGURED") {
				t.Fatal("failed verification emitted success")
			}
			if strings.Contains(string(output), "private-test-password") {
				t.Fatal("credentials leaked in output")
			}
		})
	}
}
