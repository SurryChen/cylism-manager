package cloud

import (
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/service/database"
)

func TestSwitchTargetRequiresReadySecondary(t *testing.T) {
	pods := []database.ClusterPod{
		{Name: "mysql-0", Component: "mysqld", DatabaseRole: "PRIMARY", NodeName: "node-a", Ready: true},
		{Name: "mysql-1", Component: "mysqld", DatabaseRole: "SECONDARY", NodeName: "node-b", Ready: true},
		{Name: "mysql-2", Component: "mysqld", DatabaseRole: "SECONDARY", NodeName: "node-c", Ready: false},
		{Name: "router-0", Component: "mysqlrouter", NodeName: "node-b", Ready: true},
	}
	snapshot := &database.ClusterSnapshot{Status: "ready", Pods: pods}
	if target, err := switchTarget(snapshot, "mysql-1"); err != nil || target.NodeName != "node-b" {
		t.Fatalf("ready secondary rejected: target=%+v err=%v", target, err)
	}
	for _, name := range []string{"mysql-0", "mysql-2", "router-0", "unknown"} {
		if _, err := switchTarget(snapshot, name); err == nil {
			t.Fatalf("ineligible target %s was accepted", name)
		}
	}
	snapshot.Status = "starting"
	if _, err := switchTarget(snapshot, "mysql-1"); err == nil || !strings.Contains(err.Error(), "尚未就绪") {
		t.Fatalf("unready cluster accepted: %v", err)
	}
}
