package cloud

import (
	"errors"
	"fmt"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DatabaseHandler) PauseClusterDeployment(c *gin.Context) {
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
	if item.Status != "starting" && item.Status != "error" {
		apiShared.ValidationError(c, "只有部署中或部署失败的集群可以暂停")
		return
	}
	var latest model.MySQLClusterTask
	if err := h.store.DB().Where("cluster_id = ? AND kind IN ?", item.ID, []string{"deploy", "scale_database", "scale_router", "scale_router_down"}).Order("id desc").Limit(1).Find(&latest).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if latest.Kind == "scale_database" || latest.Kind == "scale_router" {
		if latest.Status != "running" || item.Status != "starting" {
			apiShared.ValidationError(c, "本次扩容任务已结束，不能撤回已部署的实例")
			return
		}
		h.cancelMySQLScale(c, item, &latest)
		return
	}
	if latest.Kind != "deploy" || (latest.Status != "running" && latest.Status != "failed") {
		apiShared.ValidationError(c, "当前没有可暂停的初次部署任务")
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
	if err := database.PauseCluster(c.Request.Context(), client, item); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	var task model.MySQLClusterTask
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(item).Updates(map[string]any{"status": "paused", "last_error": ""}).Error; err != nil {
			return err
		}
		if err := tx.Where("cluster_id = ? AND kind = ? AND status = ?", item.ID, "deploy", "running").Order("id desc").First(&task).Error; err == nil {
			task.Status = "paused"
			task.LastProgress = "已请求 Operator 停止部署；请等待负载退出，再清理残留资源"
			if err := tx.Save(&task).Error; err != nil {
				return err
			}
			return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return nil
	})
	if err != nil {
		apiShared.DBError(c, fmt.Sprintf("已停止集群部署，但保存暂停状态失败：%v", err))
		return
	}
	item.Status, item.LastError = "paused", ""
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}

func (h *DatabaseHandler) cancelMySQLScale(c *gin.Context, item *model.MySQLCluster, task *model.MySQLClusterTask) {
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	component, target := "database", item.DatabaseInstances
	if task.Kind == "scale_router" {
		component, target = "router", database.RouterInstanceCount(item)
	}
	if err := database.CancelScale(c.Request.Context(), client, item, component, target); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	previous := target - 1
	message := fmt.Sprintf("已撤回本次 %s 扩容目标（%d → %d）；已有实例、集群资源和 PVC 保留", map[string]string{"database": "MySQL", "router": "Router"}[component], target, previous)
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": "starting", "last_error": ""}
		if component == "database" {
			updates["database_instances"] = previous
		} else {
			updates["router_instances"] = previous
		}
		if err := tx.Model(item).Updates(updates).Error; err != nil {
			return err
		}
		task.Status, task.LastProgress = "paused", message
		if err := tx.Save(task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: message}).Error
	})
	if err != nil {
		apiShared.DBError(c, fmt.Sprintf("已撤回 Operator 扩容目标，但保存平台状态失败：%v", err))
		return
	}
	if component == "database" {
		item.DatabaseInstances = previous
	} else {
		item.RouterInstances = &previous
	}
	item.Status, item.LastError = "starting", ""
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}

func (h *DatabaseHandler) ClusterCleanupState(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
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
	state, err := database.InspectClusterCleanup(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if item.Status == "paused" {
		lockValue, _ := h.mysqlSyncLocks.LoadOrStore(item.ID, &sync.Mutex{})
		lock := lockValue.(*sync.Mutex)
		lock.Lock()
		defer lock.Unlock()
		if err := h.syncRetryShutdownTask(item.ID, state); err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
	}
	apiShared.Success(c, state)
}

func (h *DatabaseHandler) syncRetryShutdownTask(clusterID uint, state *database.ClusterCleanupState) error {
	var task model.MySQLClusterTask
	err := h.store.DB().Where("cluster_id = ? AND kind = ? AND status = ?", clusterID, "retry_shutdown", "running").Order("id desc").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	message := ""
	switch {
	case !state.ClusterResource:
		task.Status = "success"
		message = "Operator 已删除 MySQL 集群资源；可继续检查并清理残留负载，数据卷保留"
	case time.Since(task.CreatedAt) >= 5*time.Minute:
		task.Status = "failed"
		message = "重试后 MySQL 集群资源仍未退出；请检查 Operator 日志和 finalizer"
	}
	if message == "" {
		return nil
	}
	task.LastProgress = message
	return h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: message}).Error
	})
}

func (h *DatabaseHandler) RetryClusterShutdown(c *gin.Context) {
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
	if item.Status != "paused" {
		apiShared.ValidationError(c, "只有正在停止负载的集群可以重试 Operator 清理")
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
	if err := database.RetryClusterShutdown(c.Request.Context(), client, item); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "retry_shutdown", Status: "running", LastProgress: "已请求重启 MySQL Operator，等待其继续清理集群资源；数据卷保留"}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, fmt.Sprintf("已请求重启 MySQL Operator，但保存任务日志失败：%v", err))
		return
	}
	apiShared.Success(c, gin.H{"task": task})
}

func (h *DatabaseHandler) CleanupPausedCluster(c *gin.Context) {
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
	if item.Status != "paused" {
		apiShared.ValidationError(c, "只有已暂停的集群可以清理残留资源")
		return
	}
	var req struct {
		ConfirmResourceName string `json:"confirm_resource_name"`
		DeleteClaims        bool   `json:"delete_claims"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ConfirmResourceName != item.ResourceName {
		apiShared.ValidationError(c, "请确认要清理的 MySQL 集群资源名称")
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
	state, err := database.CleanupPausedCluster(c.Request.Context(), client, item, req.DeleteClaims)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, state)
}
