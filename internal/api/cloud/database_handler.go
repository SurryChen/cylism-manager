package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/cylism/cylism-manager/internal/service/database/operations"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
)

type DatabaseHandler struct {
	store                 *store.Store
	key                   []byte
	client                func(context.Context, uint) (*k8s.Client, error)
	mysqlSyncLocks        sync.Map
	mysqlLifecycleWorkers sync.Map
	mysqlLifecycleLock    sync.Mutex
	registryPull          registryservice.PullExecutor
	gateways              *gateway.Service
	operations            *operations.Service
	cloudMetrics          *cloudservice.Service
	databaseSessions      sync.Map
}

func NewDatabaseHandler(s *store.Store, key []byte, client func(context.Context, uint) (*k8s.Client, error)) *DatabaseHandler {
	h := &DatabaseHandler{store: s, key: key, client: client}
	h.recoverClusterLifecycles()
	return h
}

func (h *DatabaseHandler) WithRegistryPullExecutor(executor registryservice.PullExecutor) *DatabaseHandler {
	h.registryPull = executor
	return h
}

type databaseRequest struct {
	ConnectionMode string `json:"connection_mode"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	ClusterID      uint   `json:"cluster_id"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	DatabaseName   string `json:"database_name"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	StorageSize    string `json:"storage_size"`
	StorageClass   string `json:"storage_class"`
	Image          string `json:"image"`
	NodeName       string `json:"node_name"`
	CPULimit       string `json:"cpu_limit"`
	MemoryLimit    string `json:"memory_limit"`
}

var databaseIdentifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
var environmentPrefix = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,25}$`)

func (h *DatabaseHandler) List(c *gin.Context) {
	var items []model.DatabaseInstance
	if err := h.store.DB().Order("created_at desc").Find(&items).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if items == nil {
		items = []model.DatabaseInstance{}
	}
	var gateways []model.DatabaseGateway
	if err := h.store.DB().Find(&gateways).Error; err != nil {
		apiShared.DBError(c, "读取私网网关失败")
		return
	}
	byInstance := map[uint]*model.DatabaseGateway{}
	for i := range gateways {
		gateways[i].ServiceHost = gateway.ServiceHost(gateways[i].ID)
		byInstance[gateways[i].InstanceID] = &gateways[i]
	}
	for i := range items {
		items[i].PrivateGateway = byInstance[items[i].ID]
	}
	apiShared.Success(c, items)
}

func (h *DatabaseHandler) Create(c *gin.Context) {
	var req databaseRequest
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "数据库配置无效")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.DatabaseName = strings.TrimSpace(req.DatabaseName)
	req.Username = strings.TrimSpace(req.Username)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		apiShared.ValidationError(c, "请填写实例名称，最多 128 个字符")
		return
	}
	if req.Mode != "managed" && req.Mode != "external" {
		apiShared.ValidationError(c, "请选择部署到集群或接入已有 MySQL")
		return
	}
	if req.ConnectionMode == "" {
		req.ConnectionMode = "direct"
	}
	if req.ConnectionMode != "direct" && req.ConnectionMode != "gateway" || req.Mode == "managed" && req.ConnectionMode == "gateway" {
		apiShared.ValidationError(c, "连接方式无效")
		return
	}
	if req.Mode == "external" && (req.Host == "" || req.Port < 1 || req.Port > 65535) {
		apiShared.ValidationError(c, "请填写有效的数据库地址和端口")
		return
	}
	// External endpoints can be registered and network-tested before a business
	// database exists. Preserve complete credentials from older API clients.
	hasConnection := req.DatabaseName != "" || req.Username != "" || req.Password != ""
	if hasConnection {
		if err := validateDatabaseConnection(req.DatabaseName, req.Username, req.Password); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if len(h.key) != 32 {
			apiShared.ValidationError(c, "平台加密密钥未配置")
			return
		}
	}
	if req.Mode == "managed" {
		if len(h.key) != 32 {
			apiShared.ValidationError(c, "平台加密密钥未配置")
			return
		}
		if req.CPULimit == "" && req.MemoryLimit == "" {
			req.CPULimit, req.MemoryLimit = "1", "1Gi"
		}
		var err error
		req.CPULimit, req.MemoryLimit, err = database.ValidateMySQLResources(req.CPULimit, req.MemoryLimit)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if req.ClusterID == 0 {
			apiShared.ValidationError(c, "请选择集群")
			return
		}
		if _, err := h.store.GetCluster(req.ClusterID); err != nil {
			apiShared.ValidationError(c, "集群不存在")
			return
		}
		if req.StorageSize == "" {
			req.StorageSize = "10Gi"
		}
		if strings.TrimSpace(req.Image) == "" {
			req.Image = "mysql:8.4"
		}
		if strings.ContainsAny(req.Image, " \t\n\r") {
			apiShared.ValidationError(c, "镜像地址无效")
			return
		}
		quantity, err := resource.ParseQuantity(req.StorageSize)
		if err != nil || quantity.Sign() <= 0 {
			apiShared.ValidationError(c, "存储容量无效，请填写例如 10Gi")
			return
		}
		if strings.EqualFold(req.Username, "root") {
			apiShared.ValidationError(c, "业务用户名不能使用 root")
			return
		}
		if req.Port != 0 && req.Port != 3306 {
			apiShared.ValidationError(c, "集群内 MySQL 固定使用 3306 端口")
			return
		}
	}
	var encrypted string
	if req.Password != "" {
		var err error
		encrypted, err = security.Encrypt(h.key, req.Password)
		if err != nil {
			apiShared.DBError(c, "加密数据库密码失败")
			return
		}
	}
	item := model.DatabaseInstance{ConnectionMode: req.ConnectionMode, Name: req.Name, Mode: req.Mode, ClusterID: req.ClusterID, Host: req.Host, Port: req.Port, DatabaseName: req.DatabaseName, Username: req.Username, EncryptedPassword: encrypted, StorageSize: req.StorageSize, StorageClass: strings.TrimSpace(req.StorageClass), Image: strings.TrimSpace(req.Image), Status: "registered", NodeName: strings.TrimSpace(req.NodeName), CPULimit: req.CPULimit, MemoryLimit: req.MemoryLimit}
	if req.Mode == "managed" {
		root, err := randomPassword()
		if err != nil {
			apiShared.DBError(c, "生成数据库密码失败")
			return
		}
		item.EncryptedRoot, err = security.Encrypt(h.key, root)
		if err != nil {
			apiShared.DBError(c, "加密数据库密码失败")
			return
		}
		item.Namespace, item.Port, item.Status = database.Namespace, 3306, "creating"
	}
	if item.Mode == "managed" && h.operations != nil {
		if err := h.validateStandaloneDeployment(c.Request.Context(), &item); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		err := h.store.DB().Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			item.ResourceName, item.Host = database.ResourceName(item.ID), database.ServiceHost(item.ID)
			if err := tx.Save(&item).Error; err != nil {
				return err
			}
			return h.operations.QueueDeployment(tx, &item)
		})
		if err != nil {
			apiShared.ValidationError(c, "保存实例或创建部署任务失败："+err.Error())
			return
		}
	} else {
		if item.Mode == "managed" {
			apiShared.ValidationError(c, "数据库后台任务服务未配置")
			return
		}
		if err := h.store.DB().Create(&item).Error; err != nil {
			apiShared.ValidationError(c, "数据库名称已存在或保存失败")
			return
		}
	}
	apiShared.Success(c, item)
}

func validateDatabaseConnection(name, username, password string) error {
	if !databaseIdentifier.MatchString(name) || !databaseIdentifier.MatchString(username) || password == "" {
		return fmt.Errorf("请填写数据库名、用户名和密码；数据库名及用户名只能包含英文字母、数字、下划线，并以字母开头")
	}
	return nil
}

// UpdateConnection saves existing business credentials; it never creates SQL
// databases or users and does not change the endpoint or the private gateway.
func (h *DatabaseHandler) UpdateConnection(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}

	var req struct {
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "业务连接配置无效")
		return
	}
	req.DatabaseName, req.Username = strings.TrimSpace(req.DatabaseName), strings.TrimSpace(req.Username)
	if err := validateDatabaseConnection(req.DatabaseName, req.Username, req.Password); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if len(h.key) != 32 {
		apiShared.ValidationError(c, "平台加密密钥未配置")
		return
	}
	encrypted, err := security.Encrypt(h.key, req.Password)
	if err != nil {
		apiShared.DBError(c, "加密数据库密码失败")
		return
	}
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := operations.AssertUnlocked(tx, operations.Reference(item.ID)); err != nil {
			return err
		}
		if err := tx.First(item, item.ID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.DatabaseBinding{}).Where("instance_id = ?", item.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("业务连接仍被上线模板使用，请先解除绑定，再修改并重新绑定")
		}
		item.DatabaseName, item.Username, item.EncryptedPassword = req.DatabaseName, req.Username, encrypted
		return tx.Model(item).Updates(map[string]any{"database_name": req.DatabaseName, "username": req.Username, "encrypted_password": encrypted}).Error
	})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, item)
}

func randomPassword() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (h *DatabaseHandler) get(c *gin.Context) (*model.DatabaseInstance, bool) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "数据库 ID 无效")
		return nil, false
	}
	var item model.DatabaseInstance
	if err := h.store.DB().First(&item, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "数据库不存在")
		return nil, false
	} else if err != nil {
		apiShared.DBError(c, err.Error())
		return nil, false
	}
	return &item, true
}

func (h *DatabaseHandler) apply(ctx context.Context, item *model.DatabaseInstance) error {
	if h.client == nil {
		return fmt.Errorf("集群客户端未配置")
	}
	client, err := h.client(ctx, item.ClusterID)
	if err != nil {
		return err
	}
	password, err := security.Decrypt(h.key, item.EncryptedPassword)
	if err != nil {
		return err
	}
	root, err := security.Decrypt(h.key, item.EncryptedRoot)
	if err != nil {
		return err
	}
	return database.Apply(ctx, client, item, root, password)
}

func (h *DatabaseHandler) Reconcile(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if item.Mode != "managed" {
		apiShared.ValidationError(c, "仅支持重新部署集群内 MySQL")
		return
	}
	if h.operations == nil {
		apiShared.ValidationError(c, "数据库后台任务服务未配置")
		return
	}
	err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := operations.AssertUnlocked(tx, operations.Reference(item.ID)); err != nil {
			return err
		}
		return h.operations.QueueDeployment(tx, item)
	})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) Inspect(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if item.Mode == "managed" {
		if h.client == nil {
			apiShared.K8sUnavailable(c)
			return
		}
		client, err := h.client(c.Request.Context(), item.ClusterID)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		status, err := database.Status(c.Request.Context(), client, item)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		item.Status = status
		if status == "ready" {
			item.LastError = ""
		}
		_ = h.store.DB().Model(item).Updates(map[string]any{"status": status, "last_error": item.LastError}).Error
	}
	apiShared.Success(c, item)
}

func (h *DatabaseHandler) Delete(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	if err := operations.AssertUnlocked(h.store.DB(), operations.Reference(item.ID)); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	if err := operations.AssertNoReplication(h.store.DB(), operations.Reference(item.ID)); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	var count int64
	if err := h.store.DB().Model(&model.DatabaseBinding{}).Where("instance_id = ?", item.ID).Count(&count).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if count > 0 {
		apiShared.Conflict(c, "数据库仍被上线模板使用，请先解除绑定")
		return
	}
	var privateGateway model.DatabaseGateway
	if err := h.store.DB().Where("instance_id = ?", item.ID).First(&privateGateway).Error; err == nil {
		if privateGateway.Status != "disabled" {
			apiShared.Conflict(c, "请先在私网网关中停用转发并等待任务完成")
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, "读取私网网关失败")
		return
	}
	var deleteClient *k8s.Client
	if item.Mode == "managed" {
		if h.client == nil {
			apiShared.K8sUnavailable(c)
			return
		}
		client, err := h.client(c.Request.Context(), item.ClusterID)
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		deleteClient = client
	}
	if err := h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := operations.AssertUnlocked(tx, operations.Reference(item.ID)); err != nil {
			return err
		}
		if err := operations.AssertNoReplication(tx, operations.Reference(item.ID)); err != nil {
			return err
		}
		// Acquire SQLite's write reservation before deleting Kubernetes resources.
		// A task cannot acquire an instance lock between these checks and deletion.
		if err := tx.Model(&model.DatabaseInstance{}).Where("id = ?", item.ID).UpdateColumn("id", item.ID).Error; err != nil {
			return err
		}
		var currentBindings int64
		if err := tx.Model(&model.DatabaseBinding{}).Where("instance_id = ?", item.ID).Count(&currentBindings).Error; err != nil {
			return err
		}
		if currentBindings > 0 {
			return fmt.Errorf("数据库仍被应用绑定")
		}
		var currentGateway model.DatabaseGateway
		if err := tx.Where("instance_id = ?", item.ID).First(&currentGateway).Error; err == nil {
			privateGateway = currentGateway
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if privateGateway.ID != 0 {
			if privateGateway.Status != "disabled" {
				return fmt.Errorf("请先停用私网网关")
			}
			var active int64
			if err := tx.Model(&model.DatabaseGatewayTask{}).Where("gateway_id = ? AND status IN ?", privateGateway.ID, []string{"queued", "running"}).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return fmt.Errorf("网关任务尚未完成")
			}
			subquery := tx.Model(&model.DatabaseGatewayTask{}).Select("id").Where("gateway_id = ?", privateGateway.ID)
			if err := tx.Where("task_id IN (?)", subquery).Delete(&model.DatabaseGatewayTaskLog{}).Error; err != nil {
				return err
			}
			if err := tx.Where("gateway_id = ?", privateGateway.ID).Delete(&model.DatabaseGatewayTask{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&privateGateway).Error; err != nil {
				return err
			}
		}
		if item.Mode == "managed" {
			if err := database.Delete(c.Request.Context(), deleteClient, item); err != nil {
				return err
			}
		}
		return tx.Delete(item).Error
	}); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": item.ID, "retained_pvc": func() string {
		if item.Mode == "managed" {
			return database.PVCName(item.ID)
		}
		return ""
	}()})
}

type bindingRequest struct {
	ApplicationID uint   `json:"application_id"`
	TemplateID    uint   `json:"template_id"`
	Prefix        string `json:"prefix"`
}

func (h *DatabaseHandler) ListBindings(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var bindings []model.DatabaseBinding
	if err := h.store.DB().Where("instance_id = ?", item.ID).Order("id").Find(&bindings).Error; err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	views := make([]gin.H, 0, len(bindings))
	for _, binding := range bindings {
		var app model.Application
		var template model.ApplicationDeploymentTemplate
		_ = h.store.DB().First(&app, binding.ApplicationID).Error
		_ = h.store.DB().First(&template, binding.TemplateID).Error
		views = append(views, gin.H{"id": binding.ID, "instance_id": binding.InstanceID, "application_id": binding.ApplicationID, "application_name": app.Name, "template_id": binding.TemplateID, "template_name": template.Name, "prefix": binding.Prefix, "database_name": binding.DatabaseName, "username": binding.Username})
	}
	apiShared.Success(c, views)
}

func (h *DatabaseHandler) Bind(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var req bindingRequest
	if c.ShouldBindJSON(&req) != nil || req.ApplicationID == 0 || req.TemplateID == 0 {
		apiShared.BadRequest(c, "请选择应用和上线模板")
		return
	}
	req.Prefix = strings.TrimSpace(req.Prefix)
	if req.Prefix == "" {
		req.Prefix = "DATABASE"
	}
	if !environmentPrefix.MatchString(req.Prefix) {
		apiShared.ValidationError(c, "变量前缀必须以大写字母开头，只能包含大写字母、数字、下划线")
		return
	}
	app, err := h.store.GetApplication(req.ApplicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if item.Mode == "managed" && app.Environment.ClusterID != item.ClusterID {
		apiShared.ValidationError(c, "应用与 MySQL 实例必须位于同一集群")
		return
	}
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := operations.AssertUnlocked(tx, operations.Reference(item.ID), fmt.Sprintf("template:%d", req.TemplateID)); err != nil {
			return err
		}
		var current model.DatabaseInstance
		if err := tx.First(&current, item.ID).Error; err != nil {
			return err
		}
		if current.DatabaseName == "" || current.Username == "" || current.EncryptedPassword == "" {
			return fmt.Errorf("请先配置业务数据库、用户名和密码，再绑定应用")
		}
		password, err := security.Decrypt(h.key, current.EncryptedPassword)
		if err != nil {
			return fmt.Errorf("读取数据库密码失败")
		}
		host, port, err := h.privateGatewayBinding(tx, &current, app.Environment.ClusterID)
		if err != nil {
			return err
		}
		var template model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ?", req.TemplateID, app.ID).First(&template).Error; err != nil {
			return err
		}
		var existing model.DatabaseBinding
		err = tx.Where("template_id = ?", req.TemplateID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && existing.InstanceID != item.ID {
			return fmt.Errorf("该模板已绑定其他数据库，请先解除绑定")
		}
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			return err
		}
		if spec.Config == nil {
			spec.Config = map[string]string{}
		}
		if spec.Secrets == nil {
			spec.Secrets = map[string]string{}
		}
		var secrets map[string]string
		if template.EncryptedSecrets != "" {
			plain, err := security.Decrypt(h.key, template.EncryptedSecrets)
			if err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(plain), &secrets); err != nil {
				return err
			}
		}
		if secrets == nil {
			secrets = map[string]string{}
		}
		if existing.ID != 0 && existing.Prefix != req.Prefix {
			for _, suffix := range []string{"HOST", "PORT", "NAME", "USER"} {
				delete(spec.Config, existing.Prefix+"_"+suffix)
				spec.ConfigFileKeys = withoutKey(spec.ConfigFileKeys, existing.Prefix+"_"+suffix)
			}
			delete(spec.Secrets, existing.Prefix+"_PASSWORD")
			spec.SecretFileKeys = withoutKey(spec.SecretFileKeys, existing.Prefix+"_PASSWORD")
			delete(secrets, existing.Prefix+"_PASSWORD")
		}
		keys := map[string]string{"HOST": host, "PORT": fmt.Sprint(port), "NAME": current.DatabaseName, "USER": current.Username}
		for suffix, value := range keys {
			key := req.Prefix + "_" + suffix
			if old, present := spec.Config[key]; present && old != value && (existing.ID == 0 || existing.Prefix != req.Prefix) {
				return fmt.Errorf("模板已有 %s 配置，请先处理冲突", key)
			}
			spec.Config[key] = value
			spec.ConfigDisabled = withoutKey(spec.ConfigDisabled, key)
			spec.ConfigFileKeys = withoutKey(spec.ConfigFileKeys, key)
		}
		passwordKey := req.Prefix + "_PASSWORD"
		if _, present := spec.Secrets[passwordKey]; present && (existing.ID == 0 || existing.Prefix != req.Prefix) {
			return fmt.Errorf("模板已有 %s Secret，请先处理冲突", passwordKey)
		}
		spec.Secrets[passwordKey] = ""
		spec.SecretsDisabled = withoutKey(spec.SecretsDisabled, passwordKey)
		spec.SecretFileKeys = withoutKey(spec.SecretFileKeys, passwordKey)
		secrets[passwordKey] = password
		encoded, err := json.Marshal(secrets)
		if err != nil {
			return err
		}
		template.EncryptedSecrets, err = security.Encrypt(h.key, string(encoded))
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(applicationservice.SanitizeReleaseSpec(spec))
		if err != nil {
			return err
		}
		template.Spec = string(snapshot)
		template.Revision++
		if err := tx.Save(&template).Error; err != nil {
			return err
		}
		binding := model.DatabaseBinding{ID: existing.ID, InstanceID: item.ID, TemplateID: template.ID, ApplicationID: app.ID, Prefix: req.Prefix, DatabaseName: current.DatabaseName, Username: current.Username}
		if existing.ID != 0 {
			binding.CreatedAt = existing.CreatedAt
		}
		return tx.Save(&binding).Error
	})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"application_id": app.ID, "template_id": req.TemplateID})
}

func (h *DatabaseHandler) Unbind(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	bindingID, err := apiShared.ParseID(c.Param("bindingID"))
	if err != nil {
		apiShared.BadRequest(c, "绑定 ID 无效")
		return
	}
	err = h.store.DB().Transaction(func(tx *gorm.DB) error {
		var binding model.DatabaseBinding
		if err := tx.Where("id = ? AND instance_id = ?", bindingID, item.ID).First(&binding).Error; err != nil {
			return err
		}
		if err := operations.AssertUnlocked(tx, operations.Reference(item.ID), fmt.Sprintf("template:%d", binding.TemplateID)); err != nil {
			return err
		}
		var template model.ApplicationDeploymentTemplate
		if err := tx.First(&template, binding.TemplateID).Error; err != nil {
			return err
		}
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			return err
		}
		for _, suffix := range []string{"HOST", "PORT", "NAME", "USER"} {
			delete(spec.Config, binding.Prefix+"_"+suffix)
			spec.ConfigFileKeys = withoutKey(spec.ConfigFileKeys, binding.Prefix+"_"+suffix)
		}
		delete(spec.Secrets, binding.Prefix+"_PASSWORD")
		spec.SecretFileKeys = withoutKey(spec.SecretFileKeys, binding.Prefix+"_PASSWORD")
		if template.EncryptedSecrets != "" {
			plain, err := security.Decrypt(h.key, template.EncryptedSecrets)
			if err != nil {
				return err
			}
			var secrets map[string]string
			if err := json.Unmarshal([]byte(plain), &secrets); err != nil {
				return err
			}
			delete(secrets, binding.Prefix+"_PASSWORD")
			encoded, err := json.Marshal(secrets)
			if err != nil {
				return err
			}
			if len(secrets) == 0 {
				template.EncryptedSecrets = ""
			} else {
				template.EncryptedSecrets, err = security.Encrypt(h.key, string(encoded))
				if err != nil {
					return err
				}
			}
		}
		snapshot, err := json.Marshal(applicationservice.SanitizeReleaseSpec(spec))
		if err != nil {
			return err
		}
		template.Spec = string(snapshot)
		template.Revision++
		if err := tx.Save(&template).Error; err != nil {
			return err
		}
		return tx.Delete(&binding).Error
	})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"id": bindingID})
}

func withoutKey(values []string, key string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value != key {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
