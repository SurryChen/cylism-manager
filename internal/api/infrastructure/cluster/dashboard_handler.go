package cluster

import (
	"context"
	"time"

	kubernetesapi "github.com/cylism/cylism-manager/internal/api/infrastructure/kubernetes"
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type dashboardCluster struct {
	clusterSummary
	Health    string                 `json:"health"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
	Error     string                 `json:"error,omitempty"`
	CheckedAt time.Time              `json:"checked_at,omitempty"`
}

// DashboardClusters reads each cluster independently so a failed Agent does
// not prevent the other clusters from appearing on the home page.
func (h *CatalogHandler) DashboardClusters(c *gin.Context) {
	clusters, err := h.catalog.ListClusters()
	if err != nil {
		apiShared.InternalError(c, "加载集群列表失败")
		return
	}
	rows := make([]dashboardCluster, len(clusters))
	for i := range clusters {
		rows[i] = dashboardCluster{clusterSummary: summarizeCluster(&clusters[i]), Health: "pending"}
	}
	jobs := make(chan int)
	done := make(chan struct{})
	workers := len(clusters)
	if workers > 4 {
		workers = 4
	}
	for range workers {
		go func() {
			for i := range jobs {
				rows[i] = h.dashboardCluster(c.Request.Context(), clusters[i], rows[i])
			}
			done <- struct{}{}
		}()
	}
	for i := range clusters {
		jobs <- i
	}
	close(jobs)
	for range workers {
		<-done
	}
	apiShared.Success(c, rows)
}

func (h *CatalogHandler) dashboardCluster(parent context.Context, cluster model.Cluster, row dashboardCluster) dashboardCluster {
	if cluster.ControlServerID == nil {
		return row
	}
	row.CheckedAt = time.Now().UTC()
	if h.agentClient == nil {
		row.Health, row.Error = "unavailable", "集群 Agent 访问未配置"
		return row
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	client, err := h.agentClient(ctx, cluster.ID)
	if err != nil {
		row.Health, row.Error = "unavailable", err.Error()
		return row
	}
	service := kubernetesapi.NewDashboardSummaryService(kubernetesapi.NewK8sResourceAdapter(client), 5*time.Second)
	metrics, err := service.Get(ctx)
	if err != nil {
		row.Health, row.Error = "unavailable", err.Error()
		return row
	}
	row.Metrics = metrics
	row.Health = "healthy"
	if metrics["nodes_total"] == nil && metrics["pods_total"] == nil && metrics["deployments_total"] == nil && metrics["services_total"] == nil && metrics["namespaces"] == nil {
		row.Health = "unavailable"
	} else if metrics["partial"] == true || metrics["nodes_total"] == nil {
		row.Health = "degraded"
	} else {
		for _, keys := range [][2]string{{"nodes_ready", "nodes_total"}, {"pods_ready", "pods_total"}, {"deployments_ready", "deployments_total"}} {
			if metrics[keys[0]] != metrics[keys[1]] {
				row.Health = "degraded"
				break
			}
		}
	}
	if errors, ok := metrics["errors"].(map[string]string); ok {
		for _, message := range errors {
			row.Error = message
			break
		}
	}
	return row
}
