package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DatabaseHandler) AddMySQLInstance(c *gin.Context) {
	h.scaleMySQLCluster(c, "database")
}

func (h *DatabaseHandler) AddMySQLRouter(c *gin.Context) {
	h.scaleMySQLCluster(c, "router")
}

func (h *DatabaseHandler) RetireMySQLRouter(c *gin.Context) {
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
	currentCount := database.RouterInstanceCount(item)
	if item.Status != "ready" || currentCount < 1 {
		apiShared.ValidationError(c, "请等待集群就绪且存在 Router 实例后再清退")
		return
	}
	var running int64
	if err := h.store.DB().Model(&model.MySQLClusterTask{}).Where("cluster_id = ? AND status = ? AND kind IN ?", item.ID, "running", []string{"deploy", "scale_database", "scale_router", "scale_router_down"}).Count(&running).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if running > 0 {
		apiShared.ValidationError(c, "请等待当前 MySQL 集群任务完成后再清退 Router")
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
	snapshot, err := database.Snapshot(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if snapshot.Status != "ready" || snapshot.Failure != "" {
		apiShared.ValidationError(c, "当前 MySQL 集群尚未全部就绪，请先处理异常实例")
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: "scale_router_down", Status: "running", LastProgress: fmt.Sprintf("正在将 Router 从 %d 个清退到 %d 个", currentCount, currentCount-1)}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if err := database.ScaleRouterDown(c.Request.Context(), client, item, currentCount); err != nil {
		task.Status, task.LastProgress = "failed", err.Error()
		_ = h.store.DB().Save(&task).Error
		_ = h.appendMySQLTaskLog(&task, "清退失败："+err.Error())
		apiShared.ValidationError(c, err.Error())
		return
	}
	target := currentCount - 1
	if err := h.store.DB().Model(item).Updates(map[string]any{"router_instances": target, "status": "starting", "last_error": ""}).Error; err != nil {
		task.Status = "failed"
		_ = h.store.DB().Save(&task).Error
		apiShared.DBError(c, "已提交 Operator 缩容，但保存平台目标数量失败；请检查集群资源后重试")
		return
	}
	item.RouterInstances, item.Status = &target, "starting"
	_ = h.appendMySQLTaskLog(&task, "已提交 Operator 缩容；MySQL 实例、PVC 和 Router 候选节点保留")
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}

func (h *DatabaseHandler) RepairMySQLRouterPlacement(c *gin.Context) {
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
	if item.Status != "ready" || database.RouterInstanceCount(item) == 0 {
		apiShared.ValidationError(c, "请等待 Router 就绪后再修正调度")
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
	if err := database.ValidateRouterNodes(c.Request.Context(), client, item.RouterNodes); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.RepairRouterPlacement(c.Request.Context(), client, item); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"message": "已更新 Router 调度模板，等待 Pod 滚动替换"})
}

func (h *DatabaseHandler) scaleMySQLCluster(c *gin.Context, component string) {
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
		apiShared.ValidationError(c, "请等待当前 MySQL 集群任务完成后再追加实例")
		return
	}
	var candidates []string
	if component == "database" {
		var req struct {
			CandidateNodes []string `json:"candidate_nodes"`
		}
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			apiShared.BadRequest(c, "MySQL 候选节点配置无效")
			return
		}
		var err error
		candidates, err = databaseCandidates(item.DatabaseNodes, req.CandidateNodes)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	currentCount, limit := database.DatabaseInstanceCount(item), 9
	kind, label := "scale_database", "数据库实例"
	if component == "router" {
		currentCount = database.RouterInstanceCount(item)
		if currentCount == 0 {
			var req struct {
				RouterNodes []string `json:"router_nodes"`
			}
			if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
				apiShared.BadRequest(c, "Router 节点配置无效")
				return
			}
			if len(req.RouterNodes) > 0 {
				item.RouterNodes = req.RouterNodes
			}
			if len(item.RouterNodes) == 0 {
				apiShared.ValidationError(c, "请先选择 Router 候选节点")
				return
			}
		}
		pool := make(map[string]bool, len(item.DatabaseNodes))
		for _, name := range item.DatabaseNodes {
			pool[name] = true
		}
		for _, name := range item.RouterNodes {
			if !pool[name] {
				apiShared.ValidationError(c, fmt.Sprintf("Router 候选节点 %s 不在资源池中，请先编辑资源池", name))
				return
			}
		}
		limit = len(item.RouterNodes)
		kind, label = "scale_router", "Router"
	}
	if currentCount >= limit {
		apiShared.ValidationError(c, fmt.Sprintf("已达到当前候选节点池可承载的 %s 数量上限", label))
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
	if component == "router" && currentCount == 0 {
		if err := database.ValidateRouterNodes(c.Request.Context(), client, item.RouterNodes); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	if component == "database" {
		if err := database.ValidateClusterNodes(c.Request.Context(), client, candidates, nil); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	snapshot, err := database.Snapshot(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if snapshot.Status != "ready" || snapshot.Failure != "" {
		apiShared.ValidationError(c, "当前 MySQL 集群尚未全部就绪，请先处理异常实例")
		return
	}
	task := model.MySQLClusterTask{ClusterID: item.ID, Kind: kind, Status: "running", LastProgress: fmt.Sprintf("正在将 %s 从 %d 个追加到 %d 个", label, currentCount, currentCount+1)}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return tx.Create(&model.MySQLClusterTaskLog{TaskID: task.ID, Message: task.LastProgress}).Error
	}); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	var rollbackPlacement func() error
	if component == "database" {
		rollbackPlacement, err = database.UpdateDatabaseCandidates(c.Request.Context(), client, item, candidates)
		if err != nil {
			task.Status, task.LastProgress = "failed", err.Error()
			_ = h.store.DB().Save(&task).Error
			_ = h.appendMySQLTaskLog(&task, "更新候选节点失败："+err.Error())
			apiShared.ValidationError(c, err.Error())
			return
		}
		_ = h.appendMySQLTaskLog(&task, "本次 MySQL 候选节点："+strings.Join(candidates, "、"))
	} else if currentCount == 0 {
		rollbackPlacement, err = database.PrepareFirstRouterPlacement(c.Request.Context(), client, item)
		if err != nil {
			task.Status, task.LastProgress = "failed", err.Error()
			_ = h.store.DB().Save(&task).Error
			_ = h.appendMySQLTaskLog(&task, "更新 Router 候选节点失败："+err.Error())
			apiShared.ValidationError(c, err.Error())
			return
		}
		_ = h.appendMySQLTaskLog(&task, "本次 Router 候选节点："+strings.Join(item.RouterNodes, "、"))
	}
	if err := database.ScaleCluster(c.Request.Context(), client, item, component, currentCount); err != nil {
		if rollbackPlacement != nil {
			if rollbackErr := rollbackPlacement(); rollbackErr != nil {
				err = fmt.Errorf("%w；候选节点回滚失败：%v", err, rollbackErr)
			}
		}
		task.Status, task.LastProgress = "failed", err.Error()
		_ = h.store.DB().Save(&task).Error
		_ = h.appendMySQLTaskLog(&task, "追加失败："+err.Error())
		apiShared.ValidationError(c, err.Error())
		return
	}
	updates := map[string]any{"status": "starting", "last_error": ""}
	if component == "router" {
		updates["router_instances"] = currentCount + 1
		if currentCount == 0 {
			nodes, err := json.Marshal(item.RouterNodes)
			if err != nil {
				apiShared.DBError(c, err.Error())
				return
			}
			updates["router_nodes"] = string(nodes)
		}
		count := currentCount + 1
		item.RouterInstances = &count
	} else {
		updates["database_instances"] = currentCount + 1
		item.DatabaseInstances = currentCount + 1
	}
	if err := h.store.DB().Model(item).Updates(updates).Error; err != nil {
		task.Status = "failed"
		_ = h.store.DB().Save(&task).Error
		apiShared.DBError(c, "已提交 Operator 扩容，但保存平台目标数量失败；请检查集群资源后重试")
		return
	}
	item.Status = "starting"
	_ = h.appendMySQLTaskLog(&task, "已提交 Operator 扩容，等待新增实例就绪；原有实例和 PVC 保留")
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}
