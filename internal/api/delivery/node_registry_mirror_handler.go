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
	"github.com/cylism/cylism-manager/internal/repository"
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
	service          *registryservice.MirrorService
	encKey           []byte
	verifyConnection nodeRegistryMirrorVerifier
	applyNode        registryservice.NodeMirrorApplier
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

func NewNodeRegistryMirrorHandler(repo repository.NodeRegistryMirrorRepository, encKey []byte, applyNode registryservice.NodeMirrorApplier) *NodeRegistryMirrorHandler {
	return &NodeRegistryMirrorHandler{
		service:          registryservice.NewMirrorService(repo, encKey),
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

func (h *NodeRegistryMirrorHandler) List(c *gin.Context) {
	mirrors, err := h.service.List()
	if err != nil {
		apiShared.DBError(c, "读取节点镜像源失败")
		return
	}
	model.Success(c, mirrors)
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
	model.Success(c, mirror)
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
	model.Success(c, mirror)
}

func (h *NodeRegistryMirrorHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像源 ID 无效")
		return
	}
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
	model.Success(c, gin.H{"id": id})
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
		if errorsIsRecordNotFound(err) {
			apiShared.NotFound(c, "镜像源不存在")
		} else {
			apiShared.DBError(c, "保存检测结果失败")
		}
		return
	}
	model.Success(c, mirror)
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
		apiShared.Error(c, http.StatusServiceUnavailable, model.CodeK8sUnavailable, "节点配置通道未就绪")
		return
	}
	mirror, err := h.service.StartApply(id, request.ServerIDs, h.applyNode)
	if err != nil {
		handleNodeRegistryMirrorApplyError(c, err)
		return
	}
	model.SuccessWithMessage(c, mirror, "应用任务已提交")
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
	model.Success(c, mirror)
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
