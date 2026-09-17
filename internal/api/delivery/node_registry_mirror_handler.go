package delivery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gorm.io/gorm"
)

// NodeRegistryMirrorHandler is the HTTP adapter for the node mirror service.
// It retains request parsing, user context and HTTP error mapping; validation,
// persistence and asynchronous application state live in MirrorService.
type NodeRegistryMirrorHandler struct {
	service               *registryservice.MirrorService
	encKey                []byte
	verifyConnection      nodeRegistryMirrorVerifier
	applyNode             registryservice.NodeMirrorApplier
	actualConfigInspector registryservice.NodeRegistryConfigInspectionReader
	k3sRestarter          registryservice.NodeK3sServiceRestarter
	audit                 auditservice.Repository
}

func (h *NodeRegistryMirrorHandler) WithAudit(repository auditservice.Repository) *NodeRegistryMirrorHandler {
	h.audit = repository
	return h
}

type nodeRegistryMirrorVerifier func(context.Context, *model.NodeRegistryMirror, []byte) error

type nodeRegistryMirrorApplyRequest struct {
	ServerIDs []uint `json:"server_ids"`
}

type nodeRegistryMirrorRequest struct {
	Name               string   `json:"name"`
	Registry           string   `json:"registry"`
	Endpoints          []string `json:"endpoints"`
	VerificationImage  string   `json:"verification_image"`
	Username           string   `json:"username"`
	Credential         string   `json:"credential"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	Enabled            *bool    `json:"enabled"`
}

// NewNodeRegistryMirrorHandlerWithDependencies uses a MirrorService composed
// by Bootstrap and never creates a fallback service.
func NewNodeRegistryMirrorHandlerWithDependencies(encKey []byte, applyNode registryservice.NodeMirrorApplier, service *registryservice.MirrorService) *NodeRegistryMirrorHandler {
	return &NodeRegistryMirrorHandler{
		service:          service,
		encKey:           encKey,
		verifyConnection: verifyNodeRegistryMirrorConnection,
		applyNode:        applyNode,
	}
}

func (h *NodeRegistryMirrorHandler) WithVerifier(verifier nodeRegistryMirrorVerifier) *NodeRegistryMirrorHandler {
	h.verifyConnection = verifier
	return h
}

func (h *NodeRegistryMirrorHandler) WithApplier(applier registryservice.NodeMirrorApplier) *NodeRegistryMirrorHandler {
	h.applyNode = applier
	return h
}

// WithActualConfigInspector attaches the bounded, read-only node inspection
// service composed at bootstrap.
func (h *NodeRegistryMirrorHandler) WithActualConfigInspector(inspector registryservice.NodeRegistryConfigInspectionReader) *NodeRegistryMirrorHandler {
	h.actualConfigInspector = inspector
	return h
}

func (h *NodeRegistryMirrorHandler) WithK3sRestarter(restarter registryservice.NodeK3sServiceRestarter) *NodeRegistryMirrorHandler {
	h.k3sRestarter = restarter
	return h
}

func (h *NodeRegistryMirrorHandler) List(c *gin.Context) {
	mirrors, err := h.service.List()
	if err != nil {
		apiShared.DBError(c, "读取节点镜像源失败")
		return
	}
	apiShared.Success(c, apiShared.NodeRegistryMirrorsDTO(mirrors))
}

func (h *NodeRegistryMirrorHandler) Create(c *gin.Context) {
	var request nodeRegistryMirrorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "镜像源定义无效")
		return
	}
	mirror, err := h.service.Create(mirrorInput(request), apiShared.UserID(c))
	if err != nil {
		handleNodeRegistryMirrorSaveError(c, err, false)
		return
	}
	h.recordAudit(c, "registry.mirror.create", mirror, model.AuditOutcomeSucceeded, "创建节点镜像源", nil)
	apiShared.Success(c, apiShared.NodeRegistryMirrorDTO(mirror))
}

func (h *NodeRegistryMirrorHandler) Update(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
	var request nodeRegistryMirrorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "镜像源定义无效")
		return
	}
	mirror, err := h.service.Update(id, mirrorInput(request))
	if err != nil {
		handleNodeRegistryMirrorSaveError(c, err, true)
		return
	}
	h.recordAudit(c, "registry.mirror.update", mirror, model.AuditOutcomeSucceeded, "更新节点镜像源", nil)
	apiShared.Success(c, apiShared.NodeRegistryMirrorDTO(mirror))
}

func (h *NodeRegistryMirrorHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
	mirror, _ := h.service.Get(id)
	if err := h.service.Delete(id); err != nil {
		if errorsIsRecordNotFound(err) {
			apiShared.NotFound(c, "镜像源不存在")
		} else if strings.Contains(err.Error(), "受管制品库") {
			apiShared.Conflict(c, err.Error())
		} else {
			apiShared.DBError(c, "删除节点镜像源失败")
		}
		return
	}
	h.recordAudit(c, "registry.mirror.delete", mirror, model.AuditOutcomeSucceeded, "删除节点镜像源", nil)
	apiShared.Success(c, gin.H{"id": id})
}

func (h *NodeRegistryMirrorHandler) Verify(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
	mirror, err := h.service.Verify(c.Request.Context(), id, func(ctx context.Context, mirror *model.NodeRegistryMirror) error {
		return h.verifyConnection(ctx, mirror, h.encKey)
	})
	if err != nil {
		h.recordAudit(c, "registry.mirror.verify", mirror, model.AuditOutcomeFailed, "验证节点镜像源失败", map[string]any{"error": err.Error()})
		if errorsIsRecordNotFound(err) {
			apiShared.NotFound(c, "镜像源不存在")
		} else {
			apiShared.DBError(c, "保存检测结果失败")
		}
		return
	}
	outcome, summary := model.AuditOutcomeSucceeded, "验证节点镜像源"
	metadata := map[string]any(nil)
	if mirror.LastVerifyStatus != "succeeded" {
		outcome, summary = model.AuditOutcomeFailed, "验证节点镜像源失败"
		metadata = map[string]any{"error": mirror.LastVerifyError}
	}
	h.recordAudit(c, "registry.mirror.verify", mirror, outcome, summary, metadata)
	apiShared.Success(c, apiShared.NodeRegistryMirrorDTO(mirror))
}

func (h *NodeRegistryMirrorHandler) Apply(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
	var request nodeRegistryMirrorApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "至少选择一个集群节点")
		return
	}
	if h.applyNode == nil {
		apiShared.Error(c, http.StatusServiceUnavailable, apiShared.CodeK8sUnavailable, "节点配置通道未就绪")
		return
	}
	mirror, err := h.service.StartApply(c.Request.Context(), id, request.ServerIDs, h.applyNode)
	if err != nil {
		handleNodeRegistryMirrorApplyError(c, err)
		return
	}
	h.recordAudit(c, "registry.mirror.apply", mirror, model.AuditOutcomeAccepted, "提交节点镜像源应用任务", map[string]any{"server_ids": request.ServerIDs})
	apiShared.SuccessWithMessage(c, apiShared.NodeRegistryMirrorDTO(mirror), "应用任务已提交")
}

// InspectActualConfig returns only a parsed, credential-free summary of the
// fixed K3s registries.yaml target across managed cluster nodes.
func (h *NodeRegistryMirrorHandler) InspectActualConfig(c *gin.Context) {
	if h.actualConfigInspector == nil {
		apiShared.Error(c, http.StatusServiceUnavailable, apiShared.CodeK8sUnavailable, "节点配置检查通道未就绪")
		return
	}
	var request struct {
		ServerID uint `json:"server_id"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			apiShared.BadRequest(c, "节点选择无效")
			return
		}
	}
	var inspection registryservice.NodeRegistryConfigInspection
	var err error
	if request.ServerID == 0 {
		inspection, err = h.actualConfigInspector.Inspect(c.Request.Context())
	} else {
		inspection, err = h.actualConfigInspector.InspectNode(c.Request.Context(), request.ServerID)
	}
	if err != nil {
		h.recordAudit(c, "registry.mirror.inspect-actual-config", nil, model.AuditOutcomeFailed, "检查节点实际 Registry 配置失败", nil)
		apiShared.DBError(c, "检查节点实际 Registry 配置失败")
		return
	}
	counts := map[string]int{}
	for _, node := range inspection.Nodes {
		counts[node.State]++
	}
	h.recordAudit(c, "registry.mirror.inspect-actual-config", nil, model.AuditOutcomeSucceeded, "检查节点实际 Registry 配置", map[string]any{"node_count": len(inspection.Nodes), "state_counts": counts})
	apiShared.Success(c, inspection)
}

func (h *NodeRegistryMirrorHandler) RestartNodeK3s(c *gin.Context) {
	serverID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "节点 ID 无效")
		return
	}
	if h.k3sRestarter == nil {
		apiShared.Error(c, http.StatusServiceUnavailable, apiShared.CodeK8sUnavailable, "节点 K3s 重启通道未就绪")
		return
	}
	result, err := h.k3sRestarter.Restart(c.Request.Context(), serverID)
	if err != nil {
		apiShared.ValidationError(c, "节点不是可重启的集群节点")
		return
	}
	outcome := model.AuditOutcomeSucceeded
	if result.Status != registryservice.NodeK3sRestartStatusSucceeded {
		outcome = model.AuditOutcomeFailed
	}
	h.recordNodeRestartAudit(c, serverID, outcome, result.Status)
	apiShared.Success(c, result)
}

func (h *NodeRegistryMirrorHandler) recordNodeRestartAudit(c *gin.Context, serverID uint, outcome, status string) {
	if h == nil || h.audit == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{Action: "registry.mirror.restart-node-k3s", ResourceType: "server", ResourceID: serverID, TargetName: "节点 K3s 服务", Actor: auditservice.Actor{Type: model.AuditActorUser, ID: apiShared.UserID(c), Name: apiShared.Username(c)}, Source: model.AuditSourceAPI, Outcome: outcome, Summary: "重启节点 K3s 服务", RequestID: apiShared.RequestID(c), Metadata: map[string]any{"status": status}})
}

func (h *NodeRegistryMirrorHandler) recordAudit(c *gin.Context, action string, mirror *model.NodeRegistryMirror, outcome, summary string, metadata map[string]any) {
	if h == nil || h.audit == nil {
		return
	}
	resourceID, targetName := uint(0), "节点镜像源"
	if mirror != nil {
		resourceID, targetName = mirror.ID, mirror.Name
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: action, ResourceType: "node_registry_mirror", ResourceID: resourceID, TargetName: targetName,
		Actor:  auditservice.Actor{Type: model.AuditActorUser, ID: apiShared.UserID(c), Name: apiShared.Username(c)},
		Source: model.AuditSourceAPI, Outcome: outcome, Summary: summary, RequestID: apiShared.RequestID(c), Metadata: metadata,
	})
}

func (h *NodeRegistryMirrorHandler) ApplyStatus(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
	mirror, err := h.service.Get(id)
	if err != nil {
		apiShared.NotFound(c, "镜像源不存在")
		return
	}
	apiShared.Success(c, apiShared.NodeRegistryMirrorDTO(mirror))
}

func mirrorInput(request nodeRegistryMirrorRequest) registryservice.MirrorInput {
	return registryservice.MirrorInput{Name: request.Name, Registry: request.Registry, Endpoints: request.Endpoints, VerificationImage: request.VerificationImage, Username: request.Username, Credential: request.Credential, InsecureSkipVerify: request.InsecureSkipVerify, Enabled: request.Enabled}
}

func handleNodeRegistryMirrorSaveError(c *gin.Context, err error, update bool) {
	if errorsIsRecordNotFound(err) {
		apiShared.NotFound(c, "镜像源不存在")
	} else if errors.Is(err, registryservice.ErrManagedNodeRegistryMirror) {
		apiShared.Conflict(c, err.Error())
	} else if strings.Contains(err.Error(), "名称") || strings.Contains(err.Error(), "Registry") || strings.Contains(err.Error(), "镜像") || strings.Contains(err.Error(), "凭据") {
		apiShared.ValidationError(c, err.Error())
	} else if update || err != nil {
		apiShared.Conflict(c, "镜像源名称或 Registry 地址已存在")
	}
}

func handleNodeRegistryMirrorApplyError(c *gin.Context, err error) {
	switch {
	case errorsIsRecordNotFound(err):
		apiShared.NotFound(c, "镜像源不存在")
	case errors.Is(err, registryservice.ErrMirrorDisabled), errors.Is(err, registryservice.ErrMirrorApplyRunning):
		apiShared.Conflict(c, err.Error())
	case strings.Contains(err.Error(), "节点 "), strings.Contains(err.Error(), "至少选择"), strings.Contains(err.Error(), "镜像源"), strings.Contains(err.Error(), "凭据"):
		apiShared.ValidationError(c, err.Error())
	case strings.Contains(err.Error(), "集群节点"):
		apiShared.DBError(c, "读取集群节点失败")
	default:
		apiShared.DBError(c, err.Error())
	}
}

func errorsIsRecordNotFound(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found"))
}

func verifyNodeRegistryMirrorConnection(ctx context.Context, mirror *model.NodeRegistryMirror, encKey []byte) error {
	if mirror.VerificationImage == "" {
		return fmt.Errorf("请先配置验证镜像")
	}
	var endpoints []string
	if err := json.Unmarshal([]byte(mirror.Endpoints), &endpoints); err != nil {
		return fmt.Errorf("镜像地址配置损坏")
	}
	var failures []string
	for _, endpoint := range endpoints {
		ref, err := registryservice.MirrorVerificationReference(mirror.Registry, mirror.VerificationImage, endpoint)
		if err == nil {
			err = verifyNodeRegistryMirrorEndpoint(ctx, mirror, ref, encKey)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", endpoint, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func verifyNodeRegistryMirrorEndpoint(ctx context.Context, mirror *model.NodeRegistryMirror, ref name.Reference, encKey []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	options := []remote.Option{remote.WithContext(ctx), remote.WithAuth(authn.Anonymous)}
	if mirror.Username != "" {
		if mirror.Credential == "" {
			return fmt.Errorf("已配置账号但缺少密码或 Token")
		}
		credential, err := registryservice.DecryptCredential(encKey, mirror.Credential)
		if err != nil {
			return fmt.Errorf("读取镜像源凭据失败")
		}
		options[1] = remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: mirror.Username, Password: credential}))
	}
	if mirror.InsecureSkipVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		options = append(options, remote.WithTransport(transport))
	}
	if _, err := remote.Head(ref, options...); err != nil {
		return fmt.Errorf("无法访问验证镜像: %w", err)
	}
	return nil
}
