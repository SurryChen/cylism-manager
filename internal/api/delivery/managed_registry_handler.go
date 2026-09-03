package delivery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	security "github.com/cylism/cylism-manager/internal/api/shared/security"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const managedOCIRegistryNamespace = "cylism-system"

type ManagedOCIRegistryHandler struct {
	service   *registryservice.ManagedRegistryService
	resources k8sclient.ManagedRegistryResourceReconciler
	status    k8sclient.ManagedRegistryStatusReader
	store     repository.ManagedRegistryRepository
	applyNode registryservice.NodeMirrorApplier
}

// NewManagedOCIRegistryHandlerWithDependencies uses the explicit registry
// service and Kubernetes ports composed by Bootstrap.
func NewManagedOCIRegistryHandlerWithDependencies(repo repository.ManagedRegistryRepository, resources k8sclient.ManagedRegistryResourceReconciler, status k8sclient.ManagedRegistryStatusReader, applyNode registryservice.NodeMirrorApplier, service *registryservice.ManagedRegistryService) *ManagedOCIRegistryHandler {
	return &ManagedOCIRegistryHandler{service: service, resources: resources, status: status, store: repo, applyNode: applyNode}
}

// WithResourceReconciler replaces only mutating registry convergence actions.
func (h *ManagedOCIRegistryHandler) WithResourceReconciler(resources k8sclient.ManagedRegistryResourceReconciler) *ManagedOCIRegistryHandler {
	h.resources = resources
	return h
}

// WithStatusReader replaces only registry discovery and status reads.
func (h *ManagedOCIRegistryHandler) WithStatusReader(status k8sclient.ManagedRegistryStatusReader) *ManagedOCIRegistryHandler {
	h.status = status
	return h
}

func (h *ManagedOCIRegistryHandler) List(c *gin.Context) {
	registries, err := h.service.List()
	if err != nil {
		apiShared.DBError(c, "读取受管制品库失败")
		return
	}
	for i := range registries {
		h.refreshStatus(c.Request.Context(), &registries[i])
	}
	model.Success(c, registries)
}

func (h *ManagedOCIRegistryHandler) Get(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "制品库 ID 无效")
		return
	}
	registry, err := h.service.Get(id)
	if err != nil {
		apiShared.NotFound(c, "受管制品库不存在")
		return
	}
	h.refreshStatus(c.Request.Context(), registry)
	model.Success(c, registry)
}

func (h *ManagedOCIRegistryHandler) StoragePreflight(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	registry := &model.ManagedOCIRegistry{StorageClassName: "local-path"}
	result := gin.H{"storage_class_name": registry.StorageClassName, "storage_classes": []string{registry.StorageClassName}, "data_nodes": []string{}}
	if err := h.resources.EnsureStorageClass(c.Request.Context(), registry.StorageClassName); err != nil {
		result["ready"], result["message"] = false, err.Error()
		model.Success(c, result)
		return
	}
	nodes, err := h.status.ListReadyDataNodes(c.Request.Context())
	if err != nil {
		result["ready"], result["message"] = false, "读取 Kubernetes 数据节点失败: "+security.Truncate(strings.TrimSpace(err.Error()), 512)
		model.Success(c, result)
		return
	}
	result["data_nodes"] = nodes
	if len(nodes) == 0 {
		result["ready"], result["message"] = false, "没有可用于 Registry 数据存储的 Ready Kubernetes 节点"
		model.Success(c, result)
		return
	}
	result["ready"], result["message"] = true, "local-path StorageClass 与数据节点已就绪"
	model.Success(c, result)
}

// ListEligiblePVCs exposes only the existing local-path claims that can be
// mounted by the single-replica Registry. The handler never creates PVCs.
func (h *ManagedOCIRegistryHandler) ListEligiblePVCs(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	claims, err := h.status.ListEligiblePVCs(c.Request.Context(), managedOCIRegistryNamespace)
	if err != nil {
		apiShared.Error(c, http.StatusServiceUnavailable, model.CodeK8sAPIError, "读取制品库存储卷失败: "+security.Truncate(strings.TrimSpace(err.Error()), 512))
		return
	}
	model.Success(c, claims)
}

func (h *ManagedOCIRegistryHandler) ListMatchingCertificates(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	if namespace != "" && namespace != managedOCIRegistryNamespace {
		apiShared.ValidationError(c, "制品库命名空间固定为 "+managedOCIRegistryNamespace)
		return
	}
	namespace = managedOCIRegistryNamespace
	var host string
	if endpoint := strings.TrimSpace(c.Query("endpoint")); endpoint != "" {
		_, normalizedHost, err := registryservice.NormalizeEndpoint(endpoint)
		if err != nil {
			apiShared.ValidationError(c, "访问地址无效")
			return
		}
		host = normalizedHost
	}
	certificates, err := h.status.ListMatchingCertificates(c.Request.Context(), namespace, host)
	if err != nil {
		apiShared.Error(c, http.StatusServiceUnavailable, model.CodeK8sAPIError, "读取平台证书失败: "+security.Truncate(strings.TrimSpace(err.Error()), 512))
		return
	}
	model.Success(c, certificates)
}

func (h *ManagedOCIRegistryHandler) Create(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	var request ManagedOCIRegistryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "制品库配置无效")
		return
	}
	registry, host, err := h.registryFromRequest(request, nil)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if existing, err := h.service.List(); err != nil {
		apiShared.DBError(c, "读取现有制品库失败")
		return
	} else if len(existing) > 0 {
		apiShared.Conflict(c, "首期仅支持一个受管制品库")
		return
	}
	if err := h.service.EnsureEndpointAvailable(registry.Endpoint); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	if err := h.resources.EnsureResourcesAvailable(c.Request.Context(), registry.Namespace, registry.ResourceName); err != nil {
		apiShared.Conflict(c, err.Error())
		return
	}
	if err := h.resources.ResolvePVC(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureStorageClass(c.Request.Context(), registry.StorageClassName); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureDataNode(c.Request.Context(), registry.DataNode); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureTLSCertificate(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.service.PersistCreate(registry, request.PullPassword, apiShared.UserID(c), request.ProjectIDs); err != nil {
		apiShared.Conflict(c, "保存制品库配置失败，请检查名称、地址和项目授权")
		return
	}
	if err := h.applyResources(c.Request.Context(), registry, host, request.PullPassword); err != nil {
		registry.Status, registry.LastError = "failed", security.Truncate(strings.TrimSpace(err.Error()), 512)
		_ = h.service.Save(registry)
		apiShared.InternalError(c, "部署制品库失败: "+registry.LastError)
		return
	}
	registry.Status, registry.LastError, registry.CredentialConfigured = "deploying", "", true
	_ = h.service.Save(registry)
	model.Success(c, registry)
}

func (h *ManagedOCIRegistryHandler) Update(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "制品库 ID 无效")
		return
	}
	current, err := h.service.Get(id)
	if err != nil {
		apiShared.NotFound(c, "受管制品库不存在")
		return
	}
	legacy, err := h.status.LegacyHostPath(c.Request.Context(), current)
	if err != nil {
		apiShared.InternalError(c, "检查制品库存储状态失败")
		return
	}
	if legacy {
		apiShared.Conflict(c, "现有制品库仍使用旧 hostPath 存储，请先完成 PVC 数据迁移后再更新")
		return
	}
	var request ManagedOCIRegistryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "制品库配置无效")
		return
	}
	registry, host, err := h.registryFromRequest(request, current)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if registry.Endpoint != current.Endpoint {
		apiShared.Conflict(c, "首期不支持变更制品库地址，请新建并迁移镜像")
		return
	}
	if err := h.resources.ResolvePVC(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureStorageClass(c.Request.Context(), registry.StorageClassName); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureTLSCertificate(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	password, err := h.service.ResolveUpdatePassword(registry, request.PullPassword)
	if err != nil {
		apiShared.InternalError(c, "读取或加密制品库凭据失败")
		return
	}
	if err := h.applyResources(c.Request.Context(), registry, host, password); err != nil {
		apiShared.InternalError(c, "更新制品库失败: "+security.Truncate(strings.TrimSpace(err.Error()), 512))
		return
	}
	if err := h.service.UpdateAssociations(registry, request.ProjectIDs); err != nil {
		apiShared.DBError(c, "更新关联制品库配置失败")
		return
	}
	registry.Status, registry.LastError, registry.CredentialConfigured = "deploying", "", true
	if err := h.service.Save(registry); err != nil {
		apiShared.DBError(c, "保存制品库配置失败")
		return
	}
	model.Success(c, registry)
}

// Repair reconciles every platform-owned Registry resource from the saved
// configuration. It intentionally reuses the encrypted credential instead of
// accepting a password over this operational endpoint.
func (h *ManagedOCIRegistryHandler) Repair(c *gin.Context) {
	if !h.k8sReady(c) {
		return
	}
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "制品库 ID 无效")
		return
	}
	registry, err := h.service.Get(id)
	if err != nil {
		apiShared.NotFound(c, "受管制品库不存在")
		return
	}
	legacy, err := h.status.LegacyHostPath(c.Request.Context(), registry)
	if err != nil {
		apiShared.InternalError(c, "检查制品库存储状态失败")
		return
	}
	if legacy {
		apiShared.Conflict(c, "现有制品库仍使用旧 hostPath 存储，请先完成 PVC 数据迁移后再修复")
		return
	}
	if err := h.resources.ResolvePVC(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureStorageClass(c.Request.Context(), registry.StorageClassName); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureDataNode(c.Request.Context(), registry.DataNode); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.resources.EnsureTLSCertificate(c.Request.Context(), registry); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	password, err := h.service.ResolveStoredPassword(registry)
	if err != nil {
		apiShared.InternalError(c, "读取制品库凭据失败")
		return
	}
	_, host, err := registryservice.NormalizeEndpoint(registry.Endpoint)
	if err != nil {
		apiShared.InternalError(c, "读取制品库地址失败")
		return
	}
	if err := h.applyResources(c.Request.Context(), registry, host, password); err != nil {
		registry.Status, registry.LastError = "failed", security.Truncate(strings.TrimSpace(err.Error()), 512)
		_ = h.service.Save(registry)
		apiShared.InternalError(c, "修复制品库失败: "+registry.LastError)
		return
	}
	registry.Status, registry.LastError, registry.CredentialConfigured = "deploying", "", true
	if err := h.service.Save(registry); err != nil {
		apiShared.DBError(c, "保存制品库状态失败")
		return
	}
	model.SuccessWithMessage(c, registry, "制品库资源已重新同步")
}

func (h *ManagedOCIRegistryHandler) ApplyNodeAccess(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "制品库 ID 无效")
		return
	}
	var request ManagedOCIRegistryApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.ServerIDs) == 0 {
		apiShared.BadRequest(c, "至少选择一个集群节点")
		return
	}
	registry, err := h.store.GetManagedOCIRegistry(id)
	if err != nil || registry.NodeRegistryMirrorID == nil {
		apiShared.NotFound(c, "受管制品库不存在")
		return
	}
	mirror, err := h.store.GetNodeRegistryMirror(*registry.NodeRegistryMirrorID)
	if err != nil {
		apiShared.Conflict(c, "制品库节点镜像源配置缺失")
		return
	}
	content, err := h.service.RenderNodeMirrorConfig()
	if err != nil {
		apiShared.ValidationError(c, "生成节点镜像源配置失败: "+err.Error())
		return
	}
	servers, err := h.store.ListServers()
	if err != nil {
		apiShared.DBError(c, "读取服务器失败")
		return
	}
	byID := make(map[uint]model.Server, len(servers))
	for _, server := range servers {
		if server.ClusterRole != "" {
			byID[server.ID] = server
		}
	}
	failed, seen := 0, map[uint]struct{}{}
	for _, serverID := range request.ServerIDs {
		if _, duplicate := seen[serverID]; duplicate {
			continue
		}
		seen[serverID] = struct{}{}
		server, ok := byID[serverID]
		if !ok {
			apiShared.ValidationError(c, fmt.Sprintf("节点 %d 不是可应用的集群节点", serverID))
			return
		}
		if h.applyNode == nil {
			apiShared.Error(c, http.StatusServiceUnavailable, model.CodeInternalError, "节点镜像源下发能力未初始化")
			return
		}
		status, detail := h.applyNode(c.Request.Context(), &server, content)
		if status != "success" {
			failed++
		}
		now := time.Now()
		_ = h.store.UpsertNodeRegistryMirrorStatus(&model.NodeRegistryMirrorNode{MirrorID: mirror.ID, ServerID: server.ID, Status: status, Detail: security.Truncate(strings.TrimSpace(detail), 512), AppliedAt: &now})
	}
	now := time.Now()
	mirror.LastAppliedAt = &now
	if failed == 0 {
		mirror.LastApplyStatus, mirror.LastApplyError = "succeeded", ""
	} else {
		mirror.LastApplyStatus, mirror.LastApplyError = "failed", fmt.Sprintf("%d 个节点未能应用制品库配置", failed)
	}
	_ = h.store.UpdateNodeRegistryMirror(mirror)
	updated, _ := h.store.GetManagedOCIRegistry(registry.ID)
	model.SuccessWithMessage(c, updated, "节点制品库配置已提交")
}

func (h *ManagedOCIRegistryHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "制品库 ID 无效")
		return
	}
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || !request.Confirm {
		apiShared.BadRequest(c, "删除受管制品库必须明确确认；PVC 数据不会被删除")
		return
	}
	registry, err := h.service.PrepareDelete(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "受管制品库不存在")
		} else if strings.Contains(err.Error(), "仍被") {
			apiShared.Conflict(c, err.Error())
		} else {
			apiShared.DBError(c, "检查制品库引用失败")
		}
		return
	}
	if h.status != nil && h.status.Available() && h.resources != nil {
		ctx := c.Request.Context()
		h.resources.DeleteResources(ctx, registry)
	}
	if err := h.service.DeletePersisted(registry); err != nil {
		apiShared.DBError(c, "删除受管制品库记录失败")
		return
	}
	model.SuccessWithMessage(c, gin.H{"id": id, "pvc_name": registry.PVCName}, "制品库资源已删除，PVC 数据已保留")
}

func (h *ManagedOCIRegistryHandler) k8sReady(c *gin.Context) bool {
	if h.status != nil && h.status.Available() && h.resources != nil {
		return true
	}
	apiShared.Error(c, http.StatusServiceUnavailable, model.CodeInternalError, "Kubernetes 集群未连接")
	return false
}

func (h *ManagedOCIRegistryHandler) registryFromRequest(request ManagedOCIRegistryRequest, current *model.ManagedOCIRegistry) (*model.ManagedOCIRegistry, string, error) {
	return registryservice.BuildManagedRegistry(registryservice.ManagedRegistryInput{
		Name: request.Name, Namespace: request.Namespace, Endpoint: request.Endpoint,
		VerificationImage: request.VerificationImage, RegistryImage: request.RegistryImage,
		DataNode: request.DataNode, PVCName: request.PVCName, CPURequest: request.CPURequest,
		CPULimit: request.CPULimit, MemoryRequest: request.MemoryRequest, MemoryLimit: request.MemoryLimit,
		InsecureHTTP: request.InsecureHTTP, ConfirmInsecureHTTP: request.ConfirmInsecureHTTP,
		CertificateName: request.CertificateName, PullUsername: request.PullUsername, PullPassword: request.PullPassword,
	}, current)
}

func (h *ManagedOCIRegistryHandler) applyResources(ctx context.Context, registry *model.ManagedOCIRegistry, host, password string) error {
	if h.resources == nil {
		return errors.New("Kubernetes 集群未连接")
	}
	return h.resources.Apply(ctx, registry, host, password)
}

func (h *ManagedOCIRegistryHandler) refreshStatus(ctx context.Context, registry *model.ManagedOCIRegistry) {
	if h.status == nil || !h.status.Available() {
		return
	}
	phase, status, detail := h.status.ManagedRegistryStatus(ctx, registry)
	now := time.Now()
	registry.PVCPhase, registry.Status, registry.LastError, registry.LastCheckedAt = phase, status, security.Truncate(strings.TrimSpace(detail), 512), &now
	_ = h.service.Save(registry)
}

func managedRegistryEndpointReady(ctx context.Context, endpoint string) bool {
	return k8sclient.EndpointReady(ctx, endpoint)
}
