package database

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestMemberRolesIgnoresStaleLabelsAndRetriesOtherMember(t *testing.T) {
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "database"}
	pod := func(name, ip, label string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "mysql", Labels: map[string]string{"mysql.oracle.com/cluster": "database", "component": "mysqld", "mysql.oracle.com/cluster-role": label}}, Status: corev1.PodStatus{PodIP: ip, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	}
	client := &k8s.Client{Clientset: k8sfake.NewSimpleClientset(
		pod("database-0", "10.42.1.1", "PRIMARY"), pod("database-1", "10.42.1.2", "SECONDARY"),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-root", Namespace: "mysql"}, Data: map[string][]byte{"rootPassword": []byte("private-password")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-operator-test", Namespace: "mysql-operator"}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	)}
	calls := 0
	query := func(ctx context.Context, _ *k8s.Client, operator string, input []byte, script string) ([]byte, error) {
		calls++
		var request map[string]string
		if err := json.Unmarshal(input, &request); err != nil {
			t.Fatal(err)
		}
		if _, ok := ctx.Deadline(); !ok || operator != "mysql-operator-test" || request["password"] != "private-password" || request["user"] != "root" || script != memberRolesScript {
			t.Fatal("missing bounded query or stdin credentials")
		}
		if calls == 1 {
			if request["host"] != "10.42.1.1" {
				t.Fatal("unexpected first target")
			}
			return nil, errors.New("connection failed private-password")
		}
		if request["host"] != "10.42.1.2" {
			t.Fatal("did not try other ready member")
		}
		return []byte(`CYLISM_MEMBER_ROLES=[{"host":"database-0.database-instances.mysql.svc.cluster.local","role":"SECONDARY","state":"ONLINE"},{"host":"database-1.database-instances.mysql.svc.cluster.local","role":"PRIMARY","state":"ONLINE"}]`), nil
	}
	roles, err := memberRoles(context.Background(), client, item, query)
	if err != nil || calls != 2 || len(roles) != 2 || roles[0].DatabaseRole != "SECONDARY" || roles[1].DatabaseRole != "PRIMARY" {
		t.Fatalf("stale labels overrode live roles: %+v, %v", roles, err)
	}
	roles, err = memberRoles(context.Background(), client, item, func(context.Context, *k8s.Client, string, []byte, string) ([]byte, error) {
		return []byte("private-password"), errors.New("private-password")
	})
	if roles != nil || err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("failed query leaked credentials or returned stale roles")
	}
}

func TestParseMemberRolesFailsClosed(t *testing.T) {
	item := &model.MySQLCluster{Namespace: "mysql", ResourceName: "database"}
	pods := []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "database-0"}}, {ObjectMeta: metav1.ObjectMeta{Name: "database-1"}}}
	for _, output := range []string{
		"mysql shell warning only", "CYLISM_MEMBER_ROLES=invalid", "CYLISM_MEMBER_ROLES=[]",
		`CYLISM_MEMBER_ROLES=[{"host":"database-0.evil","role":"PRIMARY","state":"ONLINE"}]`,
		`CYLISM_MEMBER_ROLES=[{"host":"database-0","role":"PRIMARY","state":"OFFLINE"}]`,
		`CYLISM_MEMBER_ROLES=[{"host":"database-0","role":"UNKNOWN","state":"ONLINE"}]`,
		`CYLISM_MEMBER_ROLES=[{"host":"database-0","role":"PRIMARY","state":"ONLINE"},{"host":"database-1","role":"PRIMARY","state":"ONLINE"}]`,
		`CYLISM_MEMBER_ROLES=[{"host":"database-0","role":"PRIMARY","state":"ONLINE"},{"host":"database-0","role":"SECONDARY","state":"ONLINE"}]`,
	} {
		if _, err := parseMemberRoles([]byte(output), item, pods); err == nil {
			t.Fatalf("unconfirmed topology accepted: %s", output)
		}
	}
	roles, err := parseMemberRoles([]byte(`CYLISM_MEMBER_ROLES=[{"host":"database-0","role":"PRIMARY","state":"ONLINE"},{"host":"database-1","role":"SECONDARY","state":"RECOVERING"}]`), item, pods)
	if err != nil || len(roles) != 2 || roles[1].DatabaseRole != "" || roles[1].MemberState != "RECOVERING" {
		t.Fatalf("recovering member must not be actionable: %+v %v", roles, err)
	}
}

func TestMemberRolesScriptQueriesMembershipWithoutChangingCluster(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 required for embedded script test")
	}
	const harness = `import sys,types
class Session:
    def run_sql(self, sql):
        assert sql == "SELECT MEMBER_HOST, MEMBER_ROLE, MEMBER_STATE FROM performance_schema.replication_group_members"
        return types.SimpleNamespace(fetch_all=lambda: [["database-0", "PRIMARY", "ONLINE"]])
    def close(self): pass
def connect(options):
    assert options["password"] == "private-password"
    if sys.argv[2] == "fail": raise RuntimeError("private-password")
    return Session()
sys.modules["mysqlsh"]=types.SimpleNamespace(globals=types.SimpleNamespace(shell=types.SimpleNamespace(open_session=connect)))
exec(sys.argv[1])
`
	for _, scenario := range []string{"success", "fail"} {
		cmd := exec.Command(python, "-c", harness, memberRolesScript, scenario)
		cmd.Stdin = strings.NewReader(`{"host":"10.42.1.1","user":"root","password":"private-password"}`)
		output, err := cmd.CombinedOutput()
		if strings.Contains(string(output), "private-password") || (scenario == "success" && (err != nil || !strings.Contains(string(output), "CYLISM_MEMBER_ROLES="))) || (scenario == "fail" && (err == nil || strings.Contains(string(output), "CYLISM_MEMBER_ROLES="))) {
			t.Fatalf("unsafe or unexpected script outcome: %s %v", output, err)
		}
	}
}
