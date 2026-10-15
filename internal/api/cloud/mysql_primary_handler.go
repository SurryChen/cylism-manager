package cloud

import (
	"context"
	"errors"
	"fmt"
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
		if !strings.EqualFold(pod.DatabaseRole, "SECONDARY") {
			return database.ClusterPod{}, fmt.Errorf("目标实例已不是副本，请刷新拓扑后重试")
		}
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
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	snapshot, err := database.Snapshot(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	target, err := switchTarget(snapshot, req.PodName)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "switch_primary", Status: "running", LastProgress: "正在将 " + target.Name + " 切换为主实例"}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 45*time.Second)
	defer cancel()
	changeErr := database.SwitchPrimary(operationCtx, client, item, target.Name, target.NodeName)
	if changeErr != nil {
		task.Status = "failed"
		switch {
		case errors.Is(changeErr, database.ErrPrimaryNotReady):
			task.LastProgress = "数据库成员尚未全部在线，暂不能切主"
		case errors.Is(changeErr, database.ErrPrimaryNotSecondary):
			task.LastProgress = "目标实例已不是副本，请刷新拓扑后重试"
		default:
			task.LastProgress = "切换主实例失败：" + changeErr.Error()
		}
	} else {
		task.Status = "success"
		task.LastProgress = "已将 " + target.Name + "（" + target.NodeName + "）切换为主实例"
	}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, "切主命令已执行，但写入任务结果失败，请刷新拓扑确认当前主实例")
		return
	}
	if changeErr != nil {
		apiShared.ValidationError(c, task.LastProgress)
		return
	}
	apiShared.Success(c, gin.H{"pod_name": target.Name, "task": task})
}
