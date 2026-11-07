package cloud

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/operations"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *DatabaseHandler) WithOperations(service *operations.Service) *DatabaseHandler {
	h.operations = service
	return h
}

func (h *DatabaseHandler) validateStandaloneDeployment(ctx context.Context, item *model.DatabaseInstance) error {
	if h.client == nil {
		return fmt.Errorf("集群客户端未配置")
	}
	client, err := h.client(ctx, item.ClusterID)
	if err != nil {
		return err
	}
	nodes, err := database.DeploymentNodes(ctx, client)
	if err != nil {
		return err
	}
	found := false
	for _, node := range nodes {
		if node.Name == item.NodeName && node.DatabaseAllowed {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("请选择一个就绪且允许数据库调度的节点")
	}
	item.StorageClass, err = database.ResolveStorageClass(ctx, client, item.StorageClass)
	return err
}

func (h *DatabaseHandler) DatabaseExecutionNodes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("cluster_id"), 10, 32)
	if err != nil || id == 0 {
		apiShared.BadRequest(c, "请选择执行集群")
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), uint(id))
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

// Admin credentials are used only by explicit capability tasks. They never
// replace business credentials and cannot be read back by the browser.
func (h *DatabaseHandler) UpdateDatabaseAdmin(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if item.Mode != "external" {
		apiShared.ValidationError(c, "平台独立 MySQL 使用部署时生成的管理凭据")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Username) == "" || len(req.Username) > 64 || req.Password == "" {
		apiShared.ValidationError(c, "请填写现有管理账号和密码")
		return
	}
	if len(h.key) != 32 {
		apiShared.ValidationError(c, "平台加密密钥未配置")
		return
	}
	encrypted, err := security.Encrypt(h.key, req.Password)
	if err != nil {
		apiShared.DBError(c, "加密管理凭据失败")
		return
	}
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if e := operations.AssertUnlocked(tx, operations.Reference(item.ID)); e != nil {
			return e
		}
		return tx.Model(item).Updates(map[string]any{"admin_username": strings.TrimSpace(req.Username), "encrypted_admin": encrypted}).Error
	})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) CreateDatabaseOperation(c *gin.Context) {
	if h.operations == nil {
		apiShared.ValidationError(c, "数据库后台任务服务未配置")
		return
	}
	var req operations.Request
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "数据库操作参数无效")
		return
	}
	task, err := h.operations.Queue(c.Request.Context(), req)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, task)
}

func (h *DatabaseHandler) ListDatabaseOperations(c *gin.Context) {
	limit := 30
	offset := 0
	if n, e := strconv.Atoi(c.Query("limit")); e == nil && n > 0 && n <= 100 {
		limit = n
	}
	if n, e := strconv.Atoi(c.Query("offset")); e == nil && n >= 0 {
		offset = n
	}
	query := h.store.DB().Model(&model.DatabaseOperation{})
	if ref := c.Query("resource"); ref != "" {
		if _, _, err := operations.ParseReference(ref); err != nil {
			apiShared.BadRequest(c, err.Error())
			return
		}
		query = query.Where("source = ? OR target = ?", ref, ref)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		apiShared.DBError(c, "读取任务数量失败")
		return
	}
	items := []model.DatabaseOperation{}
	if err := query.Order("id desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		apiShared.DBError(c, "读取任务失败")
		return
	}
	apiShared.Success(c, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}
func (h *DatabaseHandler) DatabaseOperationDetail(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("taskID"))
	if err != nil {
		apiShared.BadRequest(c, "任务 ID 无效")
		return
	}
	var task model.DatabaseOperation
	if err = h.store.DB().First(&task, id).Error; err != nil {
		apiShared.NotFound(c, "数据库任务不存在")
		return
	}
	logs := []model.DatabaseOperationLog{}
	if err = h.store.DB().Where("task_id = ?", id).Order("id").Limit(1000).Find(&logs).Error; err != nil {
		apiShared.DBError(c, "读取阶段日志失败")
		return
	}
	apiShared.Success(c, gin.H{"task": task, "logs": logs, "phases": operations.Phases(task.Kind), "snapshot_pvc": func() string {
		if task.Kind == "migrate" || task.Kind == "replicate" {
			return fmt.Sprintf("cylism-db-task-%d", task.ID)
		}
		return ""
	}()})
}
func (h *DatabaseHandler) ControlDatabaseOperation(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("taskID"))
	if err != nil {
		apiShared.BadRequest(c, "任务 ID 无效")
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if c.ShouldBindJSON(&req) != nil || h.operations == nil {
		apiShared.BadRequest(c, "数据库任务操作无效")
		return
	}
	if err = h.operations.Control(id, req.Action); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": id})
}
func (h *DatabaseHandler) ListDatabaseReplications(c *gin.Context) {
	items := []model.DatabaseReplication{}
	if err := h.store.DB().Order("id desc").Find(&items).Error; err != nil {
		apiShared.DBError(c, "读取复制关系失败")
		return
	}
	apiShared.Success(c, items)
}
