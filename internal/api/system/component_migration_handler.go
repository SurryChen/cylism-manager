package system

import (
	"fmt"
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	migration "github.com/cylism/cylism-manager/internal/service/observability/migration"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func (h *MonitoringHandler) WithMigrations(service *migration.Service, clusterID uint) *MonitoringHandler {
	h.migrations = service
	h.clusterID = clusterID
	return h
}
func (h *LoggingHandler) WithMigrations(service *migration.Service, clusterID uint) *LoggingHandler {
	h.migrations = service
	h.clusterID = clusterID
	return h
}
func (h *MonitoringHandler) ListNodeMigrations(c *gin.Context) {
	listComponentMigrations(c, h.migrations, h.clusterID, k8s.InfrastructureVictoriaMetrics)
}
func (h *LoggingHandler) ListNodeMigrations(c *gin.Context) {
	listComponentMigrations(c, h.migrations, h.clusterID, k8s.InfrastructureLoki)
}
func (h *MonitoringHandler) MigrateNode(c *gin.Context) {
	startComponentMigration(c, h.migrations, h.clusterID, k8s.InfrastructureVictoriaMetrics)
}
func (h *LoggingHandler) MigrateNode(c *gin.Context) {
	startComponentMigration(c, h.migrations, h.clusterID, k8s.InfrastructureLoki)
}

func (h *MonitoringHandler) ControlNodeMigration(c *gin.Context) {
	controlComponentMigration(c, h.migrations, h.clusterID, k8s.InfrastructureVictoriaMetrics)
}
func (h *LoggingHandler) ControlNodeMigration(c *gin.Context) {
	controlComponentMigration(c, h.migrations, h.clusterID, k8s.InfrastructureLoki)
}

func controlComponentMigration(c *gin.Context, s *migration.Service, clusterID uint, component string) {
	if s == nil || clusterID == 0 {
		apiShared.BadRequest(c, "请在指定集群的监控页面管理迁移")
		return
	}
	id, err := strconv.ParseUint(c.Param("migrationID"), 10, 32)
	if err != nil || id == 0 {
		apiShared.BadRequest(c, "迁移任务 ID 无效")
		return
	}
	var request struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "迁移操作参数无效")
		return
	}
	item, err := s.Control(clusterID, component, uint(id), request.Action)
	if err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, apiShared.APIResponse{Code: apiShared.CodeSuccess, Message: "迁移操作已记录", Data: item})
}

func listComponentMigrations(c *gin.Context, s *migration.Service, clusterID uint, component string) {
	if s == nil || clusterID == 0 {
		apiShared.BadRequest(c, "请在指定集群的监控页面管理迁移")
		return
	}
	items, err := s.List(clusterID, component)
	if err != nil {
		apiShared.InternalError(c, "读取迁移任务失败")
		return
	}
	apiShared.Success(c, items)
}
func startComponentMigration(c *gin.Context, s *migration.Service, clusterID uint, component string) {
	if s == nil || clusterID == 0 {
		apiShared.BadRequest(c, "请在指定集群的监控页面管理迁移")
		return
	}
	var request struct {
		TargetNode string `json:"target_node_name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "迁移参数无效")
		return
	}
	item, err := s.Start(c.Request.Context(), clusterID, component, request.TargetNode)
	if err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, apiShared.APIResponse{Code: apiShared.CodeSuccess, Message: "迁移任务已创建", Data: item})
}
func componentMigrationMutable(s *migration.Service, clusterID uint, component string) error {
	if s == nil || clusterID == 0 {
		return nil
	}
	items, err := s.List(clusterID, component)
	if err != nil {
		return err
	}
	for _, item := range items {
		switch item.Status {
		case "succeeded", "failed", "rolled_back", "canceled":
		default:
			return fmt.Errorf("组件迁移尚未结束，请先完成迁移或恢复任务")
		}
	}
	return nil
}
