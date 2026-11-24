package operations

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
)

func TestDiscoveryUsesSelectedNodeDespiteGatewayDiagnosticFailure(t *testing.T) {
	for _, test := range []struct {
		name, status, activeTask string
		clusterID                uint
		allowed                  bool
	}{
		{"ready", "ready", "", 1, true},
		{"partial-diagnostic", "degraded", "", 1, true},
		{"legacy-failed-diagnostic", "error", "", 1, true},
		{"disabled", "disabled", "", 1, false},
		{"deploying", "pending", "", 1, false},
		{"disabling", "disabling", "", 1, false},
		{"checking", "checking", "", 1, false},
		{"wrong-cluster", "error", "", 2, false},
		{"queued-change", "error", "queued", 1, false},
		{"running-change", "error", "running", 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := testService(t)
			password, err := security.Encrypt(s.key, "test-admin-password")
			if err != nil {
				t.Fatal(err)
			}
			source := model.DatabaseInstance{Name: "RDS", Mode: "external", ConnectionMode: "gateway", Host: "private.rds", Port: 3306, AdminUsername: "admin", EncryptedAdmin: password}
			target := model.DatabaseInstance{Name: "Replica", Mode: "managed", ClusterID: 1, Host: "replica", Port: 3306, EncryptedRoot: password}
			mustCreate(t, s, &source)
			mustCreate(t, s, &target)
			gw := model.DatabaseGateway{InstanceID: source.ID, ClusterID: test.clusterID, ServerID: 1, ListenIP: "100.80.1.1", ListenPort: 13306, Status: test.status, LastError: "another node's probe failed"}
			mustCreate(t, s, &gw)
			if test.activeTask != "" {
				mustCreate(t, s, &model.DatabaseGatewayTask{GatewayID: gw.ID, Kind: "deploy", Status: test.activeTask})
			}
			task, err := s.Queue(context.Background(), Request{Kind: "replication_discover", Source: Reference(source.ID), Target: Reference(target.ID), ClusterID: 1, NodeName: "worker"})
			if !test.allowed {
				if err == nil {
					t.Fatal("unsafe gateway state accepted")
				}
				var count int64
				s.store.DB().Model(&model.DatabaseOperation{}).Count(&count)
				if count != 0 {
					t.Fatal("rejected request created a task")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := security.Decrypt(s.key, task.EncryptedInput)
			if err != nil {
				t.Fatal(err)
			}
			var input jobInput
			if err := json.Unmarshal([]byte(raw), &input); err != nil {
				t.Fatal(err)
			}
			if input.Source.Host != gateway.ServiceHost(gw.ID) || input.Source.Port != 3306 || task.NodeName != "worker" {
				t.Fatal("discovery bypassed the private gateway or selected execution node")
			}
			var saved model.DatabaseGateway
			s.store.DB().First(&saved, gw.ID)
			if saved.Status != test.status || saved.LastError != gw.LastError {
				t.Fatal("queueing erased the gateway diagnostic")
			}
		})
	}
}
