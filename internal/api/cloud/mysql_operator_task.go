package cloud

import (
	"context"
	"fmt"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DatabaseHandler) CancelClusterOperator(c *gin.Context) {
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
	installation, err := database.OperatorInstallationStatus(c.Request.Context(), client)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if !installation.Managed || (installation.State != "installing" && installation.State != "failed" && installation.State != "cancelling") {
		apiShared.ValidationError(c, "当前没有可以中止的平台 Operator 安装任务")
		return
	}
	if err := database.CancelOperatorInstallation(c.Request.Context(), client); err != nil {
		// A partially accepted delete must still appear as cancelling on refresh.
		_, _ = h.syncMySQLOperatorTask(c.Request.Context(), item, false)
		apiShared.ValidationError(c, err.Error())
		return
	}
	if _, err := h.ensureMySQLOperatorTask(item, installation, false); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	// The Operator is shared by all logical MySQL clusters in this Kubernetes cluster.
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		var tasks []model.MySQLClusterTask
		clusterIDs := tx.Model(&model.MySQLCluster{}).Select("id").Where("cluster_id = ?", item.ClusterID)
		latestTaskIDs := tx.Model(&model.MySQLClusterTask{}).Select("MAX(id)").Where("cluster_id IN (?) AND kind = ?", clusterIDs, "install_operator").Group("cluster_id")
		if err := tx.Where("id IN (?) AND status IN ?", latestTaskIDs, []string{"running", "failed"}).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if err := tx.Model(&task).Update("status", "cancelling").Error; err != nil {
				return err
			}
			if err := tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: "用户请求中止 MySQL Operator 安装，等待 Helm 清理安装资源"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	installation, err = h.syncMySQLOperatorTask(c.Request.Context(), item, false)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, installation)
}

func (h *DatabaseHandler) UninstallClusterOperator(c *gin.Context) {
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
	var running model.MySQLClusterTask
	clusterIDs := h.store.DB().Model(&model.MySQLCluster{}).Select("id").Where("cluster_id = ?", item.ClusterID)
	if err := h.store.DB().Where("cluster_id IN (?) AND kind = ? AND status = ?", clusterIDs, "uninstall_operator", "running").Order("id desc").Limit(1).Find(&running).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if running.ID != 0 {
		apiShared.ValidationError(c, "MySQL Operator 正在卸载，请等待清理完成")
		return
	}
	if _, err := h.syncMySQLOperatorTask(c.Request.Context(), item, false); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.UninstallOperator(c.Request.Context(), client); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "uninstall_operator", Status: "running", LastProgress: "已请求 Helm 卸载 MySQL Operator，等待清理资源"}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, "已请求卸载，但保存任务失败："+err.Error())
		return
	}
	apiShared.Success(c, gin.H{"task": task})
}

// Existing platform Helm installations are recorded on first inspection as well.
func (h *DatabaseHandler) ensureMySQLOperatorTask(item *model.MySQLCluster, installation *database.OperatorInstallation, requested bool) (*model.MySQLClusterTask, error) {
	var task model.MySQLClusterTask
	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cluster_id = ? AND kind = ?", item.ID, "install_operator").Order("id desc").Limit(1).Find(&task).Error; err != nil {
			return err
		}
		if task.ID != 0 && !(requested && installation.State == "missing" && (task.Status == "success" || task.Status == "cancelled" || task.Status == "failed")) {
			return nil
		}
		if !requested && !installation.Managed {
			return nil
		}
		task = model.MySQLClusterTask{ClusterID: item.ID, Kind: "install_operator", Status: "running"}
		if !requested && !installation.CreatedAt.IsZero() {
			task.CreatedAt = installation.CreatedAt
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		message := "开始安装 MySQL Operator"
		if !requested {
			message = "检测到已有平台 Helm 安装记录，开始跟踪 MySQL Operator 安装状态"
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: message}).Error
	})
	return &task, err
}

func (h *DatabaseHandler) syncMySQLOperatorTask(ctx context.Context, item *model.MySQLCluster, requested bool) (*database.OperatorInstallation, error) {
	client, err := h.client(ctx, item.ClusterID)
	if err != nil {
		return nil, err
	}
	installation, err := database.OperatorInstallationStatus(ctx, client)
	if err != nil {
		return nil, err
	}
	uninstalling, err := h.syncMySQLOperatorUninstallTask(item, installation)
	if err != nil {
		return nil, err
	}
	if uninstalling {
		return installation, nil
	}
	task, err := h.ensureMySQLOperatorTask(item, installation, requested)
	if err != nil {
		return nil, err
	}
	if task.ID == 0 {
		return installation, nil
	}
	// Re-read under the write transaction so an in-flight poll cannot restore a
	// cancelled task using a snapshot taken before the cancellation request.
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.First(task, task.ID).Error; err != nil {
			return err
		}
		if task.Status != "running" && task.Status != "cancelling" {
			return nil
		}
		if task.Status == "cancelling" || installation.State == "cancelling" {
			task.Status = "cancelling"
			installation.State, installation.Progress = "cancelling", "已请求中止安装，等待 Helm 清理安装资源"
			if !installation.Managed {
				task.Status = "cancelled"
				installation.State, installation.Progress = "missing", "安装已中止，Helm 安装资源已清理"
			}
		} else {
			if installation.State == "ready" {
				task.Status = "success"
			}
			if installation.State == "failed" {
				task.Status = "failed"
			}
			if installation.State == "missing" && !requested && time.Since(task.CreatedAt) > time.Minute {
				task.Status = "failed"
				installation.Progress = "安装资源已不存在，MySQL Operator 未就绪"
			}
		}
		if installation.Progress != task.LastProgress {
			if err := tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: installation.Progress}).Error; err != nil {
				return err
			}
			task.LastProgress = installation.Progress
		}
		return tx.Save(task).Error
	})
	if err != nil {
		return nil, fmt.Errorf("保存 Operator 安装任务：%w", err)
	}
	return installation, nil
}

func (h *DatabaseHandler) syncMySQLOperatorUninstallTask(item *model.MySQLCluster, installation *database.OperatorInstallation) (bool, error) {
	active := false
	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		var task model.MySQLClusterTask
		clusterIDs := tx.Model(&model.MySQLCluster{}).Select("id").Where("cluster_id = ?", item.ClusterID)
		if err := tx.Where("cluster_id IN (?) AND kind = ? AND status = ?", clusterIDs, "uninstall_operator", "running").Order("id desc").Limit(1).Find(&task).Error; err != nil {
			return err
		}
		if task.ID == 0 {
			return nil
		}
		active = true
		if installation.State == "missing" {
			task.Status = "success"
			task.LastProgress = "MySQL Operator 已卸载"
			installation.Progress = task.LastProgress
		} else {
			installation.State = "cancelling"
			installation.Progress = "正在卸载 MySQL Operator，等待 Helm 清理资源"
			installation.CanUninstall = false
			return nil
		}
		if err := tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error; err != nil {
			return err
		}
		return tx.Save(&task).Error
	})
	return active, err
}
