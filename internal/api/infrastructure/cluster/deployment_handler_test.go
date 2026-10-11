package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

func TestClusterDeploymentListPaginatesWithinCluster(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, cluster := range []model.Cluster{
		{Name: "one", Distribution: "k3s", Status: "draft"},
		{Name: "two", Distribution: "k3s", Status: "draft"},
	} {
		if err := st.DB().Create(&cluster).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 23; i++ {
		operation, status := "deploy", "success"
		if i == 0 {
			operation = "agent-install"
		}
		if i == 1 {
			status = "running"
		}
		if err := st.DB().Create(&model.ClusterDeployment{ClusterID: 1, ServerID: 1, Operation: operation, Status: status}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DB().Create(&model.ClusterDeployment{ClusterID: 2, ServerID: 1, Status: "success"}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/clusters/:id/deployments", NewDeploymentHandler(st, nil).List)
	response := serve(router, newJSONRequest(http.MethodGet, "/clusters/1/deployments?limit=20&offset=20", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Items           []model.ClusterDeployment `json:"items"`
			Total           int64                     `json:"total"`
			LatestAgentTask *model.ClusterDeployment  `json:"latest_agent_task"`
			Running         bool                      `json:"running"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 23 || len(body.Data.Items) != 3 || body.Data.Items[0].ClusterID != 1 || body.Data.Items[0].ID <= body.Data.Items[2].ID || body.Data.LatestAgentTask == nil || body.Data.LatestAgentTask.Operation != "agent-install" || !body.Data.Running {
		t.Fatalf("unexpected page: %#v", body.Data)
	}
}

func TestK3sInstallCommandUsesSelectedSourceAndClusterCredentials(t *testing.T) {
	control := k3sInstallCommand("control-plane", "official", "10.0.0.1", "")
	if !strings.Contains(control, "https://get.k3s.io") || strings.Contains(control, "K3S_URL") || !strings.Contains(control, "pipefail") || !strings.Contains(control, "--tls-san 10.0.0.1") {
		t.Fatalf("control install command = %s", control)
	}
	worker := k3sInstallCommand("worker", "cn", "100.81.23.123", "secret-token")
	for _, want := range []string{"rancher-mirror.rancher.cn/k3s/k3s-install.sh", "INSTALL_K3S_MIRROR=cn", "INSTALL_K3S_SKIP_SELINUX_RPM=true", "K3S_URL=", "100.81.23.123:6443", "K3S_TOKEN=", "secret-token"} {
		if !strings.Contains(worker, want) {
			t.Fatalf("worker command missing %q", want)
		}
	}
	if !validControlAddress("100.81.23.123") || !validControlAddress("control.example.internal") || validControlAddress("https://control.example") || validControlAddress("control.example:6443") {
		t.Fatal("invalid control address validation")
	}
}

func TestDeploymentLogsAreScopedToCluster(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	first := model.Cluster{Name: "one", Distribution: "k3s", Status: "draft"}
	second := model.Cluster{Name: "two", Distribution: "k3s", Status: "draft"}
	server := model.Server{Name: "node", Host: "10.0.0.1"}
	for _, item := range []any{&first, &second, &server} {
		if err := st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	task, err := st.StartClusterDeployment(first.ID, server.ID, "control-plane", "official", server.Host, "public")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetClusterDeployment(second.ID, task.ID); err == nil {
		t.Fatal("cross-cluster deployment lookup must fail")
	}
}

func TestDeploymentAuditCorrelatesWithTaskWithoutCredentials(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	task := &model.ClusterDeployment{ID: 7, ClusterID: 9, ServerID: 3, Role: "control-plane", Mirror: "cn"}
	if err := NewDeploymentHandler(st, nil).recordAudit(task, model.AuditOutcomeAccepted, "集群部署任务已创建", "request-1"); err != nil {
		t.Fatal(err)
	}
	events, total, err := st.ListAuditLogsFiltered(model.AuditLogFilter{ResourceType: "cluster", Action: "cluster.deploy", Limit: 10})
	if err != nil || total != 1 || len(events) != 1 {
		t.Fatalf("audit events = %#v, total=%d, err=%v", events, total, err)
	}
	event := events[0]
	if event.UserID != 0 || event.ActorName != "" || event.ResourceID != 9 || event.OperationID != fmt.Sprintf("cluster-deployment-%d", task.ID) || event.Outcome != model.AuditOutcomeAccepted {
		t.Fatalf("audit event = %#v", event)
	}
	if strings.Contains(event.Detail, "token") || strings.Contains(event.Detail, "password") {
		t.Fatalf("audit detail contains credentials: %s", event.Detail)
	}
}
