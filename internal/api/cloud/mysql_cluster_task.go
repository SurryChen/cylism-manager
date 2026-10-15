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

func (h *DatabaseHandler) appendMySQLTaskLog(task *model.MySQLClusterTask, message string) error {
	if message == "" {
		return nil
	}
	return h.store.DB().Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: message}).Error
}

func (h *DatabaseHandler) syncMySQLTask(ctx context.Context, item *model.MySQLCluster) (*database.ClusterSnapshot, error) {
	lockValue, _ := h.mysqlSyncLocks.LoadOrStore(item.ID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := h.store.DB().First(item, item.ID).Error; err != nil {
		return nil, err
	}
	if item.Status == "paused" {
		return &database.ClusterSnapshot{Status: "paused", Pods: []database.ClusterPod{}}, nil
	}
	var task model.MySQLClusterTask
	if err := h.store.DB().Where("cluster_id = ? AND status = ? AND kind IN ?", item.ID, "running", []string{"deploy", "scale_database", "scale_router", "scale_router_down"}).Order("id desc").Limit(1).Find(&task).Error; err != nil {
		return nil, err
	}
	client, err := h.client(ctx, item.ClusterID)
	if err != nil {
		return nil, err
	}
	snapshot, err := database.Snapshot(ctx, client, item)
	if err != nil {
		return nil, err
	}
	if task.Kind == "scale_router_down" && snapshot.Status == "ready" {
		complete, err := database.RouterScaleDownComplete(ctx, client, item)
		if err != nil {
			return nil, err
		}
		if !complete {
			snapshot.Status = "starting"
			snapshot.Progress = fmt.Sprintf("等待 Router 从 %d 个清退到 %d 个", snapshot.RouterReady, database.RouterInstanceCount(item))
		}
	}
	if task.ID != 0 && snapshot.Failure != "" && !snapshot.FailureAt.IsZero() && snapshot.FailureAt.Before(task.CreatedAt) {
		snapshot.Failure = ""
	}
	if task.ID == 0 && item.Status == "error" && item.PreferredPrimaryNode != "" && !item.PrimaryConfigured && item.LastError != "" {
		// A failed initial primary setup remains failed until a new deployment
		// attempt is explicitly requested; readiness alone cannot erase it.
		snapshot.Failure = item.LastError
	}
	item.Status, item.OnlineInstances, item.RouterReady = snapshot.Status, snapshot.OnlineInstances, snapshot.RouterReady
	if task.ID != 0 && item.Status == "missing" {
		item.Status = "starting"
	}
	if task.ID != 0 && snapshot.Status == "ready" && snapshot.Failure == "" && item.PreferredPrimaryNode != "" && !item.PrimaryConfigured {
		preferredPod, preferredErr := database.PreferredPrimaryPod(item, snapshot.Pods)
		if preferredErr != nil {
			if err := h.store.DB().Model(item).Update("primary_configured", true).Error; err != nil {
				return nil, err
			}
			item.PrimaryConfigured = true
			if err := h.appendMySQLTaskLog(&task, "优先主节点上暂无数据库实例，保留 Operator 当前选出的主实例；可在追加实例后手动切主"); err != nil {
				return nil, err
			}
		} else {
			alreadyPrimary := false
			for _, pod := range snapshot.Pods {
				alreadyPrimary = alreadyPrimary || (pod.Name == preferredPod && strings.EqualFold(pod.DatabaseRole, "PRIMARY"))
			}
			var err error
			if !alreadyPrimary {
				err = database.ConfigurePreferredPrimary(ctx, client, item, snapshot.Pods)
			}
			if err != nil {
				if errors.Is(err, database.ErrPrimaryNotReady) {
					item.Status = "starting"
					snapshot.Status = "starting"
					snapshot.Progress = "等待数据库成员全部在线，再设置初始主实例"
				} else {
					snapshot.Failure = err.Error()
				}
			} else {
				if err := h.store.DB().Model(item).Update("primary_configured", true).Error; err != nil {
					return nil, err
				}
				item.PrimaryConfigured = true
				if err := h.appendMySQLTaskLog(&task, "已将节点 "+item.PreferredPrimaryNode+" 上的实例设为初始主实例；后续故障由集群自动切换"); err != nil {
					return nil, err
				}
				for i := range snapshot.Pods {
					if snapshot.Pods[i].Component == "mysqld" {
						if snapshot.Pods[i].NodeName == item.PreferredPrimaryNode {
							snapshot.Pods[i].DatabaseRole = "PRIMARY"
						} else {
							snapshot.Pods[i].DatabaseRole = "SECONDARY"
						}
					}
				}
			}
		}
	}
	if snapshot.Failure != "" {
		item.Status, item.LastError = "error", snapshot.Failure
		snapshot.Status = "error"
	} else if item.Status == "ready" {
		item.LastError = ""
	}
	if err := h.store.DB().Model(item).Updates(map[string]any{"status": item.Status, "online_instances": item.OnlineInstances, "router_ready": item.RouterReady, "last_error": item.LastError}).Error; err != nil {
		return nil, err
	}
	if task.ID == 0 {
		return snapshot, nil
	}
	progress := snapshot.Progress
	if task.LastProgress != progress {
		if err := h.appendMySQLTaskLog(&task, progress); err != nil {
			return nil, err
		}
		task.LastProgress = progress
	}
	if snapshot.Warning != "" && !snapshot.WarningAt.Before(task.CreatedAt) && task.LastWarning != snapshot.Warning {
		if err := h.appendMySQLTaskLog(&task, "集群事件："+snapshot.Warning); err != nil {
			return nil, err
		}
		task.LastWarning = snapshot.Warning
	}
	if item.Status == "ready" {
		task.Status = "success"
	} else if item.Status == "error" {
		task.Status = "failed"
		if err := h.appendMySQLTaskLog(&task, "任务失败："+item.LastError); err != nil {
			return nil, err
		}
	}
	if err := h.store.DB().Save(&task).Error; err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (h *DatabaseHandler) ListMySQLClusterTasks(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if h.client != nil {
		if _, err := h.syncMySQLOperatorTask(c.Request.Context(), item, false); err != nil {
			apiShared.ValidationError(c, "读取 Operator 安装进度失败："+err.Error())
			return
		}
	}
	limit, offset := apiShared.LimitOffset(c, 10, 100)
	query := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ?", item.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var uninstallPending int64
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND kind = ? AND status = ?", item.ID, "uninstall_operator", "running").Count(&uninstallPending).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var tasks []model.MySQLClusterTask
	if err := query.Order("id desc").Limit(limit).Offset(offset).Find(&tasks).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if tasks == nil {
		tasks = []model.MySQLClusterTask{}
	}
	apiShared.Success(c, gin.H{"items": tasks, "total": total, "operator_uninstall_pending": uninstallPending > 0})
}

func (h *DatabaseHandler) MySQLClusterTaskLogs(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	id, err := apiShared.ParseID(c.Param("taskID"))
	if err != nil {
		apiShared.BadRequest(c, "部署任务 ID 无效")
		return
	}
	var task model.MySQLClusterTask
	if err := h.store.DB().Where("id = ? AND cluster_id = ?", id, item.ID).First(&task).Error; err != nil {
		apiShared.NotFound(c, "部署任务不存在")
		return
	}
	var logs []model.MySQLClusterTaskLog
	if err := h.store.DB().Where("task_id = ?", task.ID).Order("id asc").Find(&logs).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if logs == nil {
		logs = []model.MySQLClusterTaskLog{}
	}
	apiShared.Success(c, logs)
}

func (h *DatabaseHandler) MySQLClusterRuntime(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status == "draft" || item.Status == "paused" {
		apiShared.Success(c, &database.ClusterSnapshot{Status: item.Status, Pods: []database.ClusterPod{}})
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	snapshot, err := h.syncMySQLTask(c.Request.Context(), item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	snapshot.Status = item.Status
	apiShared.Success(c, snapshot)
}

func (h *DatabaseHandler) failMySQLTask(task *model.MySQLClusterTask, item *model.MySQLCluster, err error) {
	item.Status, item.LastError = "error", err.Error()
	_ = h.store.DB().Model(item).Updates(map[string]any{"status": item.Status, "last_error": item.LastError}).Error
	task.Status = "failed"
	_ = h.store.DB().Save(task).Error
	_ = h.appendMySQLTaskLog(task, "部署失败："+err.Error())
}

func (h *DatabaseHandler) startMySQLTask(item *model.MySQLCluster) (*model.MySQLClusterTask, error) {
	task := &model.MySQLClusterTask{ClusterID: item.ID, Status: "running"}
	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		updated := *item
		updated.Status, updated.LastError = "starting", ""
		result := tx.Model(&model.MySQLCluster{}).
			Where("id = ? AND status IN ?", item.ID, []string{"draft", "error", "missing", "paused"}).
			Select("DatabaseNodes", "DatabaseInstances", "RouterNodes", "RouterInstances", "PreferredPrimaryNode", "PrimaryConfigured", "StorageSize", "StorageClass", "EncryptedRoot", "Status", "LastError").
			Updates(&updated)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("当前状态不能开始部署")
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		item.Status, item.LastError = "starting", ""
	}
	return task, err
}

func (h *DatabaseHandler) recoverMySQLTask(ctx context.Context, task *model.MySQLClusterTask, item *model.MySQLCluster) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, _ = h.syncMySQLTask(ctx, item)
}
