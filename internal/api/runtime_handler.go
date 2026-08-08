package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type RuntimeHandler struct {
	store    *store.Store
	encKey   []byte
	k8s      *runtime.KubernetesManager
	registry *runtime.Registry
}

type runtimeRequest struct {
	Name             string                 `json:"name"`
	RuntimeType      string                 `json:"runtime_type"`
	DeploymentMode   string                 `json:"deployment_mode"`
	RuntimeVersion   string                 `json:"runtime_version"`
	Image            string                 `json:"image"`
	Namespace        string                 `json:"namespace"`
	EndpointURL      string                 `json:"endpoint_url"`
	Port             int32                  `json:"port"`
	HealthPath       string                 `json:"health_path"`
	PVCName          string                 `json:"pvc_name"`
	Storage          string                 `json:"storage"`
	StorageClassName string                 `json:"storage_class_name"`
	NodeName         string                 `json:"node_name"`
	ModelName        string                 `json:"model_name"`
	ModelBaseURL     string                 `json:"model_base_url"`
	APIStyle         string                 `json:"api_style"`
	APIKey           *string                `json:"api_key"`
	Config           map[string]interface{} `json:"config"`
}

func NewRuntimeHandler(s *store.Store, encKey []byte, k8sManager *runtime.KubernetesManager, registries ...*runtime.Registry) *RuntimeHandler {
	registry := runtime.BuiltinRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	} else if k8sManager != nil && k8sManager.Registry != nil {
		registry = k8sManager.Registry
	}
	return &RuntimeHandler{store: s, encKey: encKey, k8s: k8sManager, registry: registry}
}

func (h *RuntimeHandler) Catalog(c *gin.Context) {
	model.Success(c, h.registry.List())
}

func (h *RuntimeHandler) List(c *gin.Context) {
	runtimes, err := h.store.ListRuntimes()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "读取 Runtime 列表失败")
		return
	}
	for index := range runtimes {
		h.sanitize(&runtimes[index])
	}
	model.Success(c, runtimes)
}

func (h *RuntimeHandler) Get(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return
	}
	instance, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return
	}
	h.sanitize(instance)
	model.Success(c, instance)
}

func (h *RuntimeHandler) Create(c *gin.Context) {
	var req runtimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime 定义无效")
		return
	}
	instance, err := h.instanceFromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	instance.CreatedBy = getUserID(c)
	if err := h.store.CreateRuntime(instance); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Runtime 名称已存在")
		return
	}
	h.sanitize(instance)
	model.SuccessWithMessage(c, instance, "Runtime 已创建")
}

func (h *RuntimeHandler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return
	}
	current, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return
	}
	var req runtimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime 定义无效")
		return
	}
	updated, err := h.instanceFromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	updated.ID = current.ID
	updated.CreatedAt = current.CreatedAt
	updated.CreatedBy = current.CreatedBy
	updated.Status = current.Status
	updated.DesiredGeneration = current.DesiredGeneration + 1
	if err := h.store.UpdateRuntime(updated); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "Runtime 名称已存在或配置无效")
		return
	}
	h.sanitize(updated)
	model.SuccessWithMessage(c, updated, "Runtime 配置已更新，请部署或更新 Runtime")
}

func (h *RuntimeHandler) Deploy(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return
	}
	instance, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return
	}
	if h.k8s == nil {
		if instance.DeploymentMode != model.RuntimeDeploymentExternal {
			model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
			return
		}
	}
	if instance.DeploymentMode == model.RuntimeDeploymentExternal {
		if strings.TrimSpace(instance.EndpointURL) == "" {
			model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "外部 Runtime 必须提供连接地址")
			return
		}
		checker := h.k8s
		if checker == nil {
			checker = runtime.NewKubernetesManager(nil)
		}
		status, detail := checker.Health(c.Request.Context(), instance)
		instance.Status, instance.HealthStatus, instance.HealthDetail = status, status, detail
		instance.ObservedGeneration = instance.DesiredGeneration
		_ = h.store.UpdateRuntime(instance)
		h.sanitize(instance)
		if status != model.RuntimeStatusReady {
			model.ErrorWithData(c, http.StatusBadGateway, model.CodeK8sAPIError, detail, instance)
			return
		}
		model.SuccessWithMessage(c, instance, "外部 Runtime 已连接")
		return
	}
	apiKey := ""
	if instance.EncryptedAPIKey != "" {
		apiKey, err = crypto.Decrypt(h.encKey, instance.EncryptedAPIKey)
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取 Runtime 模型凭据失败")
			return
		}
	}
	if strings.TrimSpace(apiKey) == "" {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "托管 Runtime 必须配置模型 API 密钥")
		return
	}
	instance.Status = model.RuntimeStatusDeploying
	if err := h.store.UpdateRuntime(instance); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存 Runtime 状态失败")
		return
	}
	runtimeAPIKey, err := h.runtimeAPIKey(instance)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "读取 Runtime API 凭据失败")
		return
	}
	if err := h.k8s.Apply(c.Request.Context(), instance, apiKey, runtimeAPIKey); err != nil {
		instance.Status = model.RuntimeStatusFailed
		instance.HealthDetail = err.Error()
		_ = h.store.UpdateRuntime(instance)
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, err.Error())
		return
	}
	instance.Status = model.RuntimeStatusDeploying
	if ready, readyErr := h.k8s.DeploymentReady(c.Request.Context(), instance); readyErr == nil && ready {
		instance.Status = model.RuntimeStatusReady
	}
	instance.ObservedGeneration = instance.DesiredGeneration
	if err := h.store.UpdateRuntime(instance); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存 Runtime 部署状态失败")
		return
	}
	h.sanitize(instance)
	model.SuccessWithMessage(c, instance, "Runtime 已部署或更新")
}

func (h *RuntimeHandler) Health(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return
	}
	instance, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return
	}
	if h.k8s == nil && instance.DeploymentMode != model.RuntimeDeploymentExternal {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	checker := h.k8s
	if checker == nil {
		checker = runtime.NewKubernetesManager(nil)
	}
	status, detail := checker.Health(c.Request.Context(), instance)
	if err := h.store.UpdateRuntimeHealth(instance.ID, status, detail, time.Now()); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存健康检查结果失败")
		return
	}
	instance.HealthStatus, instance.HealthDetail, instance.LastHealthAt = status, detail, ptrTime(time.Now())
	if status == model.RuntimeStatusReady {
		instance.Status = model.RuntimeStatusReady
	} else if instance.Status != model.RuntimeStatusUninstalled {
		instance.Status = model.RuntimeStatusDegraded
	}
	_ = h.store.UpdateRuntime(instance)
	h.sanitize(instance)
	model.Success(c, instance)
}

func (h *RuntimeHandler) Uninstall(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Runtime ID 无效")
		return
	}
	instance, err := h.store.GetRuntime(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Runtime 不存在")
		return
	}
	if h.k8s == nil && instance.DeploymentMode != model.RuntimeDeploymentExternal {
		model.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "Kubernetes 集群未连接")
		return
	}
	if instance.DeploymentMode == model.RuntimeDeploymentExternal {
		instance.Status = model.RuntimeStatusUninstalled
		if err := h.store.UpdateRuntime(instance); err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存卸载状态失败")
			return
		}
		h.sanitize(instance)
		model.SuccessWithMessage(c, gin.H{"runtime": instance, "pvc_deleted": false}, "外部 Runtime 已解除连接")
		return
	}
	deletePVC := c.Query("delete_data") == "true"
	if err := h.k8s.Delete(c.Request.Context(), instance, deletePVC); err != nil {
		model.Error(c, http.StatusBadGateway, model.CodeK8sAPIError, err.Error())
		return
	}
	instance.Status = model.RuntimeStatusUninstalled
	instance.HealthStatus = ""
	instance.HealthDetail = ""
	if err := h.store.UpdateRuntime(instance); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, "保存卸载状态失败")
		return
	}
	h.sanitize(instance)
	model.SuccessWithMessage(c, gin.H{"runtime": instance, "pvc_deleted": deletePVC}, "Runtime 已卸载")
}

func (h *RuntimeHandler) instanceFromRequest(req runtimeRequest, current *model.RuntimeInstance) (*model.RuntimeInstance, error) {
	name := strings.TrimSpace(req.Name)
	if current != nil && name == "" {
		name = current.Name
	}
	if !runtime.RuntimeNameValid(name) {
		return nil, fmt.Errorf("Runtime 名称必须是合法的 Kubernetes 名称")
	}
	runtimeType := strings.TrimSpace(req.RuntimeType)
	if current != nil && runtimeType == "" {
		runtimeType = current.RuntimeType
	}
	adapter, ok := h.registry.Get(runtimeType)
	if !ok {
		return nil, fmt.Errorf("不支持的 Runtime 类型: %s", runtimeType)
	}
	image := strings.TrimSpace(req.Image)
	if current != nil && image == "" {
		image = current.Image
	}
	if image == "" && strings.TrimSpace(req.DeploymentMode) == model.RuntimeDeploymentExternal {
		image = "external"
	}
	if image == "" {
		return nil, fmt.Errorf("Runtime 镜像不能为空")
	}
	namespace := strings.TrimSpace(req.Namespace)
	if current != nil && namespace == "" {
		namespace = current.Namespace
	}
	if namespace == "" {
		namespace = runtime.DefaultNamespace
	}
	if !runtime.NamespaceValid(namespace) {
		return nil, fmt.Errorf("Runtime 命名空间无效")
	}
	deploymentMode := strings.TrimSpace(req.DeploymentMode)
	if deploymentMode == "" && current != nil {
		deploymentMode = current.DeploymentMode
	}
	if deploymentMode == "" {
		deploymentMode = model.RuntimeDeploymentManaged
	}
	if deploymentMode != model.RuntimeDeploymentManaged && deploymentMode != model.RuntimeDeploymentExternal {
		return nil, fmt.Errorf("Runtime 部署模式无效")
	}
	if current != nil && current.DeploymentMode != "" && current.DeploymentMode != deploymentMode && current.Status != model.RuntimeStatusDraft && current.Status != model.RuntimeStatusUninstalled {
		return nil, fmt.Errorf("已部署 Runtime 不能直接切换部署模式，请先卸载")
	}
	port := req.Port
	if port == 0 && current != nil {
		port = current.Port
	}
	if port == 0 {
		port = runtime.RuntimeDefaultPort
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("Runtime 端口必须在 1 到 65535 之间")
	}
	healthPath := strings.TrimSpace(req.HealthPath)
	if healthPath == "" && current != nil {
		healthPath = current.HealthPath
	}
	if healthPath == "" {
		healthPath = "/health"
	}
	if !strings.HasPrefix(healthPath, "/") || strings.ContainsAny(healthPath, "\r\n") {
		return nil, fmt.Errorf("健康检查路径无效")
	}
	storage := strings.TrimSpace(req.Storage)
	if storage == "" && current != nil {
		storage = current.Storage
	}
	if storage == "" {
		storage = "10Gi"
	}
	apiStyle := strings.TrimSpace(req.APIStyle)
	if apiStyle == "" && current != nil {
		apiStyle = current.APIStyle
	}
	if apiStyle == "" {
		apiStyle = "responses"
	}
	modelName := strings.TrimSpace(req.ModelName)
	modelBaseURL := strings.TrimSpace(req.ModelBaseURL)
	endpointURL := ""
	if current != nil {
		if modelName == "" {
			modelName = current.ModelName
		}
		if modelBaseURL == "" {
			modelBaseURL = current.ModelBaseURL
		}
		endpointURL = current.EndpointURL
	}
	if strings.TrimSpace(req.EndpointURL) != "" {
		endpointURL = strings.TrimSpace(req.EndpointURL)
	}
	if deploymentMode == model.RuntimeDeploymentExternal && endpointURL == "" {
		return nil, fmt.Errorf("外部 Runtime 必须提供连接地址")
	}
	if deploymentMode == model.RuntimeDeploymentManaged {
		definition := adapter.Definition()
		port = definition.DefaultPort
		healthPath = definition.DefaultHealthPath
	}
	if req.Config == nil && current != nil && current.Config != "" {
		if err := json.Unmarshal([]byte(current.Config), &req.Config); err != nil {
			return nil, fmt.Errorf("读取已有 Runtime 配置失败")
		}
	}
	configBytes, err := json.Marshal(req.ConfigOrEmpty())
	if err != nil {
		return nil, fmt.Errorf("Runtime 配置无效")
	}
	instance := &model.RuntimeInstance{Name: name, RuntimeType: runtimeType, DeploymentMode: deploymentMode, RuntimeVersion: strings.TrimSpace(req.RuntimeVersion), Image: image, Namespace: namespace, Port: port, HealthPath: healthPath, PVCName: strings.TrimSpace(req.PVCName), Storage: storage, StorageClassName: strings.TrimSpace(req.StorageClassName), NodeName: strings.TrimSpace(req.NodeName), EndpointURL: endpointURL, ModelName: modelName, ModelBaseURL: modelBaseURL, APIStyle: apiStyle, Config: string(configBytes), Status: model.RuntimeStatusDraft}
	if current != nil {
		if instance.PVCName == "" {
			instance.PVCName = current.PVCName
		}
		if instance.StorageClassName == "" {
			instance.StorageClassName = current.StorageClassName
		}
		if instance.NodeName == "" {
			instance.NodeName = current.NodeName
		}
		instance.SecretName = current.SecretName
		instance.EncryptedAPIKey = current.EncryptedAPIKey
		instance.EncryptedRuntimeAPIKey = current.EncryptedRuntimeAPIKey
	}
	if req.APIKey != nil {
		if strings.TrimSpace(*req.APIKey) == "" {
			instance.EncryptedAPIKey = ""
		} else {
			instance.EncryptedAPIKey, err = crypto.Encrypt(h.encKey, *req.APIKey)
			if err != nil {
				return nil, fmt.Errorf("保存 Runtime 模型凭据失败")
			}
		}
	}
	if err := h.registry.Validate(instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (r runtimeRequest) ConfigOrEmpty() map[string]interface{} {
	if r.Config == nil {
		return map[string]interface{}{}
	}
	return r.Config
}

func (h *RuntimeHandler) sanitize(instance *model.RuntimeInstance) {
	instance.APIKeyConfigured = instance.EncryptedAPIKey != ""
	instance.EncryptedAPIKey = ""
}

func (h *RuntimeHandler) runtimeAPIKey(instance *model.RuntimeInstance) (string, error) {
	if instance.EncryptedRuntimeAPIKey != "" {
		return crypto.Decrypt(h.encKey, instance.EncryptedRuntimeAPIKey)
	}
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	plain := base64.RawURLEncoding.EncodeToString(value)
	encrypted, err := crypto.Encrypt(h.encKey, plain)
	if err != nil {
		return "", err
	}
	instance.EncryptedRuntimeAPIKey = encrypted
	if err := h.store.UpdateRuntime(instance); err != nil {
		return "", err
	}
	return plain, nil
}

func ptrTime(t time.Time) *time.Time { return &t }
