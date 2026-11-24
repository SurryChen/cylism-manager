package cloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mysqlPrimarySwitchRequest struct {
	PodName string `json:"pod_name"`
}

func switchTarget(snapshot *database.ClusterSnapshot, name string) (database.ClusterPod, error) {
	if snapshot.Status != "ready" || snapshot.Failure != "" {
		return database.ClusterPod{}, fmt.Errorf("MySQL 集群尚未就绪，暂不能切换主实例")
	}
	for _, pod := range snapshot.Pods {
		if pod.Name != name {
			continue
		}
		if pod.Component != "mysqld" || !pod.Ready || pod.NodeName == "" {
			return database.ClusterPod{}, fmt.Errorf("请选择已就绪的数据库副本实例")
		}
		// Role labels may lag after failover. Validate the live member role in
		// MySQL Shell, including the already-primary idempotent case.
		return pod, nil
	}
	return database.ClusterPod{}, fmt.Errorf("目标数据库实例不存在，请刷新拓扑后重试")
}

func (h *DatabaseHandler) SwitchClusterPrimary(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	var req mysqlPrimarySwitchRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.PodName) == "" {
		apiShared.BadRequest(c, "请选择要切换为主实例的数据库副本")
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
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
		apiShared.ValidationError(c, "MySQL 集群尚未就绪，暂不能切换主实例")
		return
	}
	if err := h.expirePrimarySwitchTasks(item.ID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var running int64
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND status = ?", item.ID, "running").Count(&running).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if running > 0 {
		apiShared.Conflict(c, "请等待当前集群任务完成后再切主")
		return
	}
	podName := strings.TrimSpace(req.PodName)
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "switch_primary", Status: "running", LastProgress: "已创建切主任务，目标实例：" + podName}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	// Persist before dispatch. Cluster I/O runs outside both the HTTP request
	// lifetime and the cluster query lock so progress remains readable.
	go h.runPrimarySwitchTask(task.ID, item.ID, podName)
	c.JSON(http.StatusAccepted, apiShared.APIResponse{Code: apiShared.CodeSuccess, Message: "切主任务已创建，正在后台执行", Data: gin.H{"pod_name": podName, "task": task}})
}

func (h *DatabaseHandler) runPrimarySwitchTask(taskID, clusterID uint, podName string) {
	defer func() {
		if recover() != nil {
			h.recordPrimarySwitchProgress(taskID, "failed", "切主执行异常，请核对实际主实例后再重试")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if !h.recordPrimarySwitchProgress(taskID, "running", "正在检查集群状态与目标副本："+podName) {
		return
	}
	var item model.MySQLCluster
	if err := h.store.DB().First(&item, clusterID).Error; err != nil {
		h.recordPrimarySwitchProgress(taskID, "failed", "切换主实例失败：读取集群记录失败")
		return
	}
	client, err := h.client(ctx, item.ClusterID)
	if err != nil {
		h.recordPrimarySwitchProgress(taskID, "failed", "切换主实例失败："+err.Error())
		return
	}
	snapshot, err := database.Snapshot(ctx, client, &item)
	if err != nil {
		h.recordPrimarySwitchProgress(taskID, "failed", "检查集群状态失败："+err.Error())
		return
	}
	target, err := switchTarget(snapshot, podName)
	if err != nil {
		h.recordPrimarySwitchProgress(taskID, "failed", err.Error())
		return
	}
	if !h.recordPrimarySwitchProgress(taskID, "running", "正在执行主实例切换并验证结果："+target.Name+"（"+target.NodeName+"）") {
		return
	}
	changeErr := database.SwitchPrimary(ctx, client, &item, target.Name, target.NodeName)
	status := "success"
	progress := "已验证 " + target.Name + "（" + target.NodeName + "）为主实例"
	if changeErr != nil {
		status = "failed"
		switch {
		case errors.Is(changeErr, database.ErrPrimaryNotReady):
			progress = "数据库成员尚未全部在线，暂不能切主"
		case errors.Is(changeErr, database.ErrPrimaryRoleUnknown):
			progress = "无法确认目标实例的实时成员角色，请检查组复制状态"
		default:
			progress = "切换主实例失败：" + changeErr.Error() + "；请核对实际主实例后再重试"
		}
	}
	h.recordPrimarySwitchProgress(taskID, status, progress)
}

// Update only running tasks. A late worker must not overwrite timeout recovery.
func (h *DatabaseHandler) recordPrimarySwitchProgress(taskID uint, status, progress string) bool {
	updated := false
	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.MySQLClusterTask{}).Where("id = ? AND kind = ? AND status = ?", taskID, "switch_primary", "running").Updates(map[string]any{"status": status, "last_progress": progress})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		updated = true
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: taskID, Message: progress}).Error
	})
	if err != nil {
		log.Printf("persist MySQL primary switch task %d: %v", taskID, err)
	}
	return err == nil && updated
}

// Never replay a switch after a process interruption: it may already have
// changed the primary. Keep a durable failure explaining the uncertain outcome.
func (h *DatabaseHandler) expirePrimarySwitchTasks(clusterID uint) error {
	return h.store.DB().Transaction(func(tx *gorm.DB) error {
		var tasks []model.MySQLClusterTask
		if err := tx.Where("cluster_id = ? AND kind = ? AND status = ? AND created_at < ?", clusterID, "switch_primary", "running", time.Now().Add(-2*time.Minute)).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			message := "切主任务超时或被平台重启中断，执行结果未确认；请刷新拓扑核对实际主实例后再重试"
			result := tx.Model(&model.MySQLClusterTask{}).Where("id = ? AND status = ?", task.ID, "running").Updates(map[string]any{"status": "failed", "last_progress": message})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				if err := tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: message}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
