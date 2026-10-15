package cloud

import (
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
)

func (h *DatabaseHandler) MySQLClusterConfig(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status == "draft" || item.Status == "paused" {
		apiShared.Success(c, gin.H{"mycnf": item.MyCNF, "restart_required": false})
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	value, pending, err := database.MyCNFState(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"mycnf": value, "restart_required": pending})
}

func (h *DatabaseHandler) UpdateMySQLClusterConfig(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	var req struct {
		MyCNF string `json:"mycnf"`
	}
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "MySQL 配置无效")
		return
	}
	value, err := database.ValidateMyCNF(req.MyCNF)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	lockValue, _ := h.mysqlSyncLocks.LoadOrStore(item.ID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := h.store.DB().First(item, item.ID).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if item.Status != "draft" && item.Status != "ready" {
		apiShared.ValidationError(c, "仅待部署或运行中的集群可修改 MySQL 配置")
		return
	}
	var running int64
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND status = ?", item.ID, "running").Count(&running).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if running > 0 {
		apiShared.ValidationError(c, "请等待当前集群任务完成后再修改配置")
		return
	}
	if item.Status == "ready" {
		if h.client == nil {
			apiShared.K8sUnavailable(c)
			return
		}
		client, err := h.client(c.Request.Context(), item.ClusterID)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if err := database.UpdateMyCNF(c.Request.Context(), client, item, value); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	restartRequired := item.Status == "ready" && (item.ConfigRestartRequired || value != item.MyCNF)
	if err := h.store.DB().Model(item).Updates(map[string]any{"my_cnf": value, "config_restart_required": restartRequired}).Error; err != nil {
		apiShared.DBError(c, "Operator 配置已更新，但平台保存失败，请刷新后重试")
		return
	}
	item.MyCNF = value
	item.ConfigRestartRequired = restartRequired
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) RestartMySQLClusterForConfig(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	lockValue, _ := h.mysqlSyncLocks.LoadOrStore(item.ID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := h.store.DB().First(item, item.ID).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if item.Status != "ready" {
		apiShared.ValidationError(c, "请等待集群就绪后再重启 MySQL 实例")
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	actual, pending, err := database.MyCNFState(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if !pending || actual != item.MyCNF {
		apiShared.ValidationError(c, "MySQL 配置没有待应用的修改，或平台记录与 Operator 配置不一致")
		return
	}
	if err := database.RestartMySQLForConfig(c.Request.Context(), client, item, actual); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.store.DB().Model(item).Update("config_restart_required", false).Error; err != nil {
		apiShared.DBError(c, "已提交 MySQL 滚动重启，但保存平台状态失败；请检查实例状态")
		return
	}
	apiShared.Success(c, gin.H{"message": "已提交 MySQL StatefulSet 滚动重启"})
}
