package cloud

import (
	"context"
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

func lifecycleStatus(status string) bool {
	return status == "stopping" || status == "resuming" || status == "retiring"
}

func (h *DatabaseHandler) ClusterLifecycle(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	var req struct {
		Action              string `json:"action"`
		ConfirmResourceName string `json:"confirm_resource_name"`
		DeleteClaims        bool   `json:"delete_claims"`
		DeleteSecrets       bool   `json:"delete_secrets"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ConfirmResourceName != item.ResourceName {
		apiShared.ValidationError(c, "请输入集群资源名称以确认操作")
		return
	}
	if req.Action != "retire" && (req.DeleteClaims || req.DeleteSecrets) {
		apiShared.ValidationError(c, "仅清退操作可以清理数据卷或密钥")
		return
	}
	target := map[string]string{"stop": "stopping", "resume": "resuming", "retire": "retiring"}[req.Action]
	if target == "" {
		apiShared.ValidationError(c, "不支持的集群操作")
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
	allowed := map[string][]string{"stop": {"ready", "error", "stopping"}, "resume": {"stopped", "resuming", "stopping"}, "retire": {"stopped", "paused", "missing", "retiring", "retired"}}
	valid := false
	for _, status := range allowed[req.Action] {
		if status == item.Status {
			valid = true
		}
	}
	if !valid {
		apiShared.ValidationError(c, "当前状态不允许此操作，请刷新后重试")
		return
	}
	var running int64
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND status IN ?", item.ID, []string{"running", "cancelling"}).Count(&running).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if running > 0 {
		apiShared.ValidationError(c, "集群已有后台任务，请等待完成")
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
	if err := database.ValidateClusterLifecycle(c.Request.Context(), client, item, req.Action); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: req.Action, Status: "running", DeleteClaims: req.DeleteClaims, DeleteSecrets: req.DeleteSecrets, LastProgress: map[string]string{"stop": "已创建停用任务，保留原集群和数据卷", "resume": "已创建恢复任务，沿用原集群配置、凭据和数据卷", "retire": "已创建清退任务，移除集群负载，保留数据卷和根密钥"}[req.Action]}
	if req.Action == "retire" {
		claims, secrets := "保留", "保留"
		if req.DeleteClaims {
			claims = "清理"
		}
		if req.DeleteSecrets {
			secrets = "清理"
		}
		task.LastProgress = fmt.Sprintf("已创建清退任务；数据卷：%s；密钥：%s。等待集群负载完全退出后处理", claims, secrets)
	}
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(item).Updates(map[string]any{"status": target, "last_error": ""}).Error; err != nil {
			return err
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	})
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	item.Status, item.LastError = target, ""
	h.runClusterLifecycle(item.ID)
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}

// Tasks are durable and resumed at server startup. Each Kubernetes attempt has
// a bounded request context; the overall lifecycle has no arbitrary deadline.
func (h *DatabaseHandler) recoverClusterLifecycles() {
	if h.client == nil {
		return
	}
	var tasks []model.MySQLClusterTask
	if h.store.DB().Where("status = ? AND kind IN ?", "running", []string{"stop", "resume", "retire"}).Find(&tasks).Error != nil {
		return
	}
	for _, task := range tasks {
		h.runClusterLifecycle(task.ClusterID)
	}
}
func (h *DatabaseHandler) runClusterLifecycle(id uint) {
	if _, loaded := h.mysqlLifecycleWorkers.LoadOrStore(id, true); loaded {
		return
	}
	go func() {
		defer func() {
			h.mysqlLifecycleWorkers.Delete(id)
			// A new task may have been accepted between the previous task's commit
			// and this worker exiting. Never leave that task waiting for a restart.
			var pending int64
			if h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND status = ? AND kind IN ?", id, "running", []string{"stop", "resume", "retire"}).Count(&pending).Error == nil && pending > 0 {
				h.runClusterLifecycle(id)
			}
		}()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			done := h.clusterLifecycleStep(ctx, id)
			cancel()
			if done {
				return
			}
			timer := time.NewTimer(5 * time.Second)
			<-timer.C
		}
	}()
}
func (h *DatabaseHandler) clusterLifecycleStep(ctx context.Context, id uint) bool {
	lockValue, _ := h.mysqlSyncLocks.LoadOrStore(id, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	// Serialize changes to the shared controller, even for different records.
	h.mysqlLifecycleLock.Lock()
	defer h.mysqlLifecycleLock.Unlock()
	var item model.MySQLCluster
	if err := h.store.DB().First(&item, id).Error; err != nil {
		return errors.Is(err, gorm.ErrRecordNotFound)
	}
	var task model.MySQLClusterTask
	if err := h.store.DB().Where("cluster_id = ? AND status = ? AND kind IN ?", id, "running", []string{"stop", "resume", "retire"}).First(&task).Error; err != nil {
		return errors.Is(err, gorm.ErrRecordNotFound)
	}
	client, err := h.client(ctx, item.ClusterID)
	done, progress, nextStatus := false, "", item.Status
	if err == nil {
		switch task.Kind {
		case "stop":
			done, progress, err = database.StopClusterStep(ctx, client, &item)
			if done {
				nextStatus = "stopped"
			}
		case "resume":
			err = database.ResumeCluster(ctx, client, &item)
			if err == nil {
				var operatorState string
				operatorState, err = database.OperatorState(ctx, client)
				if err == nil && operatorState != "ready" {
					progress = "已恢复目标副本，等待 MySQL Operator 就绪"
				}
			}
			if err == nil && progress == "" {
				var snapshot *database.ClusterSnapshot
				snapshot, err = database.Snapshot(ctx, client, &item)
				if err == nil && snapshot.Failure != "" {
					err = fmt.Errorf("恢复后的集群异常：%s", snapshot.Failure)
				}
				if err == nil {
					databaseReady, routerReady := 0, 0
					for _, pod := range snapshot.Pods {
						if !pod.Ready {
							continue
						}
						if pod.Component == "mysqld" {
							databaseReady++
						} else if pod.Component == "mysqlrouter" {
							routerReady++
						}
					}
					done, progress = snapshot.Status == "ready" && databaseReady >= database.DatabaseInstanceCount(&item) && routerReady >= database.RouterInstanceCount(&item), snapshot.Progress
					if !done && snapshot.Status == "ready" {
						progress = "等待恢复后的 MySQL 和 Router Pod 实际就绪"
					}
					item.OnlineInstances, item.RouterReady = snapshot.OnlineInstances, snapshot.RouterReady
					if done {
						nextStatus = "ready"
					}
				}
			}
		case "retire":
			done, progress, err = database.RetireClusterStep(ctx, client, &item)
			if done && err == nil {
				done, progress, err = database.CleanupRetainedClusterResources(ctx, client, &item, task.DeleteClaims, task.DeleteSecrets)
			}
			if done && err == nil {
				nextStatus = "retired"
			}
		}
	}
	if err != nil {
		progress = fmt.Sprintf("操作失败，可修复原因后重试：%v", err)
		task.Status = "failed"
		item.LastError = err.Error()
		done = true
	}
	if done && err == nil {
		task.Status = "success"
		item.LastError = ""
	}
	if nextStatus == "stopped" || nextStatus == "retired" {
		item.OnlineInstances, item.RouterReady = 0, 0
	}
	dbErr := h.store.DB().Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": nextStatus, "last_error": item.LastError, "online_instances": item.OnlineInstances, "router_ready": item.RouterReady}
		if done && err == nil && task.Kind == "retire" && task.DeleteSecrets {
			updates["encrypted_root"] = ""
		}
		if err := tx.Model(&item).Updates(updates).Error; err != nil {
			return err
		}
		if progress != task.LastProgress {
			if err := tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: progress}).Error; err != nil {
				return err
			}
			task.LastProgress = progress
		}
		return tx.Save(&task).Error
	})
	return done && dbErr == nil
}
