package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
)

type mysqlClusterCreateRequest struct {
	Name      string `json:"name"`
	ClusterID uint   `json:"cluster_id"`
}

type mysqlClusterTopologyRequest struct {
	DatabaseNodes     []string `json:"database_nodes"`
	CandidateNodes    []string `json:"candidate_nodes"`
	DatabaseInstances int      `json:"database_instances"`
	RouterNodes       []string `json:"router_nodes"`
	RouterInstances   *int     `json:"router_instances"`
	StorageSize       string   `json:"storage_size"`
	StorageClass      string   `json:"storage_class"`
}

func (h *DatabaseHandler) ListClusters(c *gin.Context) {
	var items []model.MySQLCluster
	if err := h.store.DB().Order("created_at desc").Find(&items).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if items == nil {
		items = []model.MySQLCluster{}
	}
	apiShared.Success(c, items)
}

func (h *DatabaseHandler) CreateCluster(c *gin.Context) {
	var req mysqlClusterCreateRequest
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "MySQL 集群信息无效")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 || req.ClusterID == 0 {
		apiShared.ValidationError(c, "请填写集群名称并选择 Kubernetes 集群")
		return
	}
	if _, err := h.store.GetCluster(req.ClusterID); err != nil {
		apiShared.ValidationError(c, "Kubernetes 集群不存在")
		return
	}
	item := model.MySQLCluster{Name: req.Name, ClusterID: req.ClusterID, Namespace: database.Namespace, Status: "draft", DatabaseNodes: []string{}, RouterNodes: []string{}}
	if err := h.store.DB().Create(&item).Error; err != nil {
		apiShared.ValidationError(c, "MySQL 集群名称已存在或保存失败")
		return
	}
	item.ResourceName = database.ClusterResourceName(item.ID)
	if err := h.store.DB().Model(&item).Update("resource_name", item.ResourceName).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) getCluster(c *gin.Context) (*model.MySQLCluster, bool) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "MySQL 集群 ID 无效")
		return nil, false
	}
	var item model.MySQLCluster
	if err := h.store.DB().First(&item, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "MySQL 集群不存在")
		return nil, false
	} else if err != nil {
		apiShared.DBError(c, err.Error())
		return nil, false
	}
	return &item, true
}

func (h *DatabaseHandler) ClusterNodes(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	nodes, err := database.DeploymentNodes(c.Request.Context(), client)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, nodes)
}

func (h *DatabaseHandler) UpdateClusterResourcePool(c *gin.Context) {
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
	if item.Status != "draft" && item.Status != "paused" && item.Status != "ready" {
		apiShared.ValidationError(c, "请等待当前 MySQL 集群任务完成后再修改资源池")
		return
	}
	var req struct {
		DatabaseNodes []string `json:"database_nodes"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.DatabaseNodes) < 1 || len(req.DatabaseNodes) > 9 {
		apiShared.ValidationError(c, "请选择 1–9 台数据库候选节点")
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
	if err := database.ValidateClusterNodes(c.Request.Context(), client, req.DatabaseNodes, nil); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if sameNodePool(item.DatabaseNodes, req.DatabaseNodes) {
		apiShared.Success(c, item)
		return
	}
	if item.Status == "ready" && len(item.RouterNodes) > 0 {
		currentPool := make(map[string]bool, len(item.DatabaseNodes))
		updatedPool := make(map[string]bool, len(req.DatabaseNodes))
		for _, name := range item.DatabaseNodes {
			currentPool[name] = true
		}
		for _, name := range req.DatabaseNodes {
			updatedPool[name] = true
		}
		for _, name := range item.RouterNodes {
			if currentPool[name] && !updatedPool[name] {
				apiShared.ValidationError(c, fmt.Sprintf("节点 %s 仍在 Router 候选节点中，不能移出资源池", name))
				return
			}
		}
	}
	encoded, err := json.Marshal(req.DatabaseNodes)
	if err != nil {
		apiShared.DBError(c, "保存数据库资源池失败")
		return
	}
	updates := map[string]any{"database_nodes": string(encoded)}
	if item.PreferredPrimaryNode != "" && database.ValidatePrimaryNode(item.PreferredPrimaryNode, req.DatabaseNodes) != nil {
		updates["preferred_primary_node"] = ""
	}
	var rollback func() error
	if item.Status == "ready" {
		rollback, err = database.UpdateDatabaseResourcePool(c.Request.Context(), client, item, req.DatabaseNodes)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
	}
	result := h.store.DB().Model(item).Where("status = ?", item.Status).Updates(updates)
	if result.Error != nil {
		if rollback != nil {
			if err := rollback(); err != nil {
				apiShared.DBError(c, "保存数据库资源池失败，调度规则回滚失败："+err.Error())
				return
			}
		}
		apiShared.DBError(c, "保存数据库资源池失败")
		return
	}
	if result.RowsAffected == 0 {
		if rollback != nil {
			if err := rollback(); err != nil {
				apiShared.DBError(c, "集群状态已变化，调度规则回滚失败："+err.Error())
				return
			}
		}
		apiShared.ValidationError(c, "集群状态已变化，请刷新后重试")
		return
	}
	if err := h.store.DB().First(item, item.ID).Error; err != nil {
		apiShared.DBError(c, "读取数据库资源池失败")
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) ClusterOperator(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	installation, err := h.syncMySQLOperatorTask(c.Request.Context(), item, false)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, installation)
}

func (h *DatabaseHandler) InstallClusterOperator(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	cluster, err := h.store.GetCluster(item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, "Kubernetes 集群不存在")
		return
	}
	if !strings.EqualFold(cluster.Distribution, "k3s") {
		apiShared.ValidationError(c, "仅 K3s 集群支持平台安装 MySQL Operator")
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
	if installation.State == "cancelling" {
		apiShared.ValidationError(c, "正在清理上次安装资源，请等待中止完成后再安装")
		return
	}
	task, err := h.ensureMySQLOperatorTask(item, installation, true)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if err := database.InstallOperator(c.Request.Context(), client); err != nil {
		task.Status = "failed"
		_ = h.store.DB().Save(task).Error
		_ = h.appendMySQLTaskLog(task, "安装失败："+err.Error())
		apiShared.ValidationError(c, err.Error())
		return
	}
	installation, err = h.syncMySQLOperatorTask(c.Request.Context(), item, false)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.store.DB().First(task, task.ID).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"state": installation.State, "task": task})
}

func (h *DatabaseHandler) DeployCluster(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status != "draft" && item.Status != "error" && item.Status != "missing" && item.Status != "paused" {
		apiShared.ValidationError(c, "已部署的集群不能通过创建流程修改拓扑")
		return
	}
	var req mysqlClusterTopologyRequest
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "部署配置无效")
		return
	}
	if len(req.DatabaseNodes) > 0 && !sameNodePool(item.DatabaseNodes, req.DatabaseNodes) {
		apiShared.ValidationError(c, "数据库候选节点与资源池不一致，请先修改资源池")
		return
	}
	if len(item.DatabaseNodes) == 0 {
		apiShared.ValidationError(c, "请先添加数据库资源池")
		return
	}
	req.DatabaseNodes = item.DatabaseNodes
	candidates, err := databaseCandidates(item.DatabaseNodes, req.CandidateNodes)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	req.RouterNodes = nil
	zeroRouters := 0
	req.RouterInstances = &zeroRouters
	req.StorageSize = strings.TrimSpace(req.StorageSize)
	req.StorageClass = item.StorageClass
	if req.StorageSize == "" {
		req.StorageSize = "10Gi"
	}
	quantity, err := resource.ParseQuantity(req.StorageSize)
	if err != nil || quantity.Sign() <= 0 || len(req.StorageClass) > 128 {
		apiShared.ValidationError(c, "存储配置无效")
		return
	}
	if h.client == nil || len(h.key) != 32 {
		apiShared.ValidationError(c, "集群客户端或平台加密密钥未配置")
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if item.Status == "paused" {
		state, err := database.InspectClusterCleanup(c.Request.Context(), client, item)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if state.ClusterResource || len(state.Workloads) > 0 || len(state.Pods) > 0 || len(state.Claims) > 0 {
			apiShared.ValidationError(c, "旧集群资源仍在清理中；请先清理残留负载和数据卷，再重新部署")
			return
		}
	}
	if err := database.CheckOperator(c.Request.Context(), client, item.Namespace); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.ValidateClusterNodes(c.Request.Context(), client, candidates, req.RouterNodes); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := database.ValidateDatabaseInstances(req.DatabaseInstances, req.DatabaseNodes); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	routerInstances := len(req.RouterNodes)
	if req.RouterInstances != nil {
		routerInstances = *req.RouterInstances
	}
	if routerInstances < 0 || routerInstances > len(req.RouterNodes) {
		apiShared.ValidationError(c, "Router 实例数须在 0 和候选节点数之间")
		return
	}
	storageClass, err := database.ResolveStorageClass(c.Request.Context(), client, req.StorageClass)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	item.DatabaseNodes, item.RouterNodes = req.DatabaseNodes, req.RouterNodes
	item.DatabaseInstances = req.DatabaseInstances
	item.RouterInstances = &routerInstances
	item.StorageSize, item.StorageClass = req.StorageSize, storageClass
	item.PreferredPrimaryNode, item.PrimaryConfigured = "", false
	var root string
	if item.EncryptedRoot == "" {
		root, err = randomPassword()
		if err == nil {
			item.EncryptedRoot, err = security.Encrypt(h.key, root)
		}
	} else {
		root, err = security.Decrypt(h.key, item.EncryptedRoot)
	}
	if err != nil {
		apiShared.DBError(c, "准备数据库密钥失败")
		return
	}
	task, err := h.startMySQLTask(item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	_ = h.appendMySQLTaskLog(task, "已验证 MySQL Operator、节点和存储配置；本次候选节点："+strings.Join(candidates, "、"))
	deployment := *item
	deployment.DatabaseNodes = candidates
	if err := database.ApplyCluster(c.Request.Context(), client, &deployment, root); err != nil {
		h.failMySQLTask(task, item, err)
		apiShared.ValidationError(c, "MySQL 集群部署失败："+err.Error())
		return
	}
	_ = h.appendMySQLTaskLog(task, "已提交 MySQL 集群资源，等待 Operator 创建目标实例")
	item.Status, item.LastError = "starting", ""
	_ = h.store.DB().Model(item).Updates(map[string]any{"status": item.Status, "last_error": ""}).Error
	apiShared.Success(c, gin.H{"cluster": item, "task": task})
}

func sameNodePool(saved, requested []string) bool {
	if len(saved) != len(requested) {
		return false
	}
	remaining := make(map[string]bool, len(saved))
	for _, node := range saved {
		remaining[node] = true
	}
	for _, node := range requested {
		if !remaining[node] {
			return false
		}
		delete(remaining, node)
	}
	return len(remaining) == 0
}

func databaseCandidates(pool, selected []string) ([]string, error) {
	if len(selected) == 0 {
		selected = pool
	}
	if len(selected) < 1 || len(selected) > 9 {
		return nil, fmt.Errorf("请选择 1–9 台资源池内的 MySQL 候选节点")
	}
	allowed := make(map[string]bool, len(pool))
	for _, name := range pool {
		allowed[name] = true
	}
	seen := make(map[string]bool, len(selected))
	for _, name := range selected {
		if !allowed[name] || seen[name] {
			return nil, fmt.Errorf("MySQL 候选节点 %s 不在资源池中或重复选择", name)
		}
		seen[name] = true
	}
	return append([]string(nil), selected...), nil
}

func (h *DatabaseHandler) InspectCluster(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status == "draft" || item.Status == "paused" {
		apiShared.Success(c, item)
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	_, err := h.syncMySQLTask(c.Request.Context(), item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) DeleteCluster(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status != "draft" {
		apiShared.ValidationError(c, "已部署的 MySQL 集群需先完成数据备份和卸载，此处仅能删除未部署的记录")
		return
	}
	if err := h.store.DB().Delete(item).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": item.ID})
}
