package applicationapi

import (
	"context"
	"fmt"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
)

func (h *ApplicationHandler) WithClusterClientFactory(factory func(context.Context, uint) (*k8s.Client, error)) *ApplicationHandler {
	h.clusterClient = factory
	return h
}

func (h *ApplicationHandler) forCluster(ctx context.Context, clusterID uint) (*ApplicationHandler, error) {
	if h.clusterClient == nil {
		return h, nil
	}
	if clusterID == 0 {
		return nil, fmt.Errorf("环境尚未绑定集群，请先在项目与环境中选择集群")
	}
	client, err := h.clusterClient(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("连接环境所属集群失败: %w", err)
	}
	adapter := NewKubernetesAdapter(client)
	if adapter == nil {
		return nil, fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	scoped := *h
	scoped.kubernetes = adapter
	scoped.workflow = applicationservice.NewReleaseService(h.resources, h.encKey, adapter)
	scoped.workloads = applicationservice.NewWorkloadService(h.applications, adapter)
	return &scoped, nil
}

func (h *ApplicationHandler) forApplication(ctx context.Context, id uint) (*ApplicationHandler, *model.Application, error) {
	app, err := h.queries.GetApplication(id)
	if err != nil {
		return nil, nil, err
	}
	scoped, err := h.forCluster(ctx, app.Environment.ClusterID)
	return scoped, app, err
}

func (h *ApplicationHandler) HandleApplication(action func(*ApplicationHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := apiShared.ParseID(c.Param("id"))
		if err != nil {
			apiShared.BadRequest(c, "应用 ID 无效")
			return
		}
		scoped, _, err := h.forApplication(c.Request.Context(), id)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		action(scoped, c)
	}
}

func (h *ApplicationHandler) HandleEnvironment(action func(*ApplicationHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := apiShared.ParseID(c.Param("projectID"))
		if err != nil {
			apiShared.BadRequest(c, "项目 ID 无效")
			return
		}
		environmentID, err := apiShared.ParseID(c.Param("environmentID"))
		if err != nil {
			apiShared.BadRequest(c, "环境 ID 无效")
			return
		}
		environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
		if err != nil {
			apiShared.NotFound(c, "环境不存在")
			return
		}
		scoped, err := h.forCluster(c.Request.Context(), environment.ClusterID)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		action(scoped, c)
	}
}

func (h *ApplicationHandler) HandleWorkspace(action func(*ApplicationHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := apiShared.ParseID(c.Query("project_id"))
		if err != nil {
			apiShared.BadRequest(c, "项目 ID 无效")
			return
		}
		environmentID, err := apiShared.ParseID(c.Query("environment_id"))
		if err != nil {
			apiShared.BadRequest(c, "环境 ID 无效")
			return
		}
		environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
		if err != nil {
			apiShared.ValidationError(c, "环境不属于所选项目")
			return
		}
		scoped, err := h.forCluster(c.Request.Context(), environment.ClusterID)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		action(scoped, c)
	}
}

func (h *ApplicationHandler) runtimeInfos(ctx context.Context, applications []model.Application, releases map[uint][]model.Release) map[uint]applicationservice.RuntimeInfo {
	if h.clusterClient == nil {
		return h.queries.ApplicationRuntimeInfos(ctx, h.kubernetes, applications, releases)
	}
	byCluster := make(map[uint][]model.Application)
	for _, app := range applications {
		byCluster[app.Environment.ClusterID] = append(byCluster[app.Environment.ClusterID], app)
	}
	result := make(map[uint]applicationservice.RuntimeInfo, len(applications))
	for clusterID, items := range byCluster {
		var reader applicationservice.RuntimeReader
		if scoped, err := h.forCluster(ctx, clusterID); err == nil {
			reader = scoped.kubernetes
		}
		for id, runtime := range h.queries.ApplicationRuntimeInfos(ctx, reader, items, releases) {
			result[id] = runtime
		}
	}
	return result
}
