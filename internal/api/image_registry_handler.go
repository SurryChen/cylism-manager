package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type ImageRegistryHandler struct {
	store            repository.ImageRegistryRepository
	encKey           []byte
	verifyConnection registryConnectionVerifier
}

type registryConnectionVerifier func(context.Context, *model.ImageRegistry, []byte) error

type imageRegistryRequest struct {
	Name              string  `json:"name"`
	Endpoint          string  `json:"endpoint"`
	VerificationImage string  `json:"verification_image"`
	AuthType          string  `json:"auth_type"`
	Username          string  `json:"username"`
	Credential        *string `json:"credential"`
	Enabled           *bool   `json:"enabled"`
	ProjectIDs        []uint  `json:"project_ids"`
}

func NewImageRegistryHandler(s repository.ImageRegistryRepository, encKey []byte) *ImageRegistryHandler {
	return &ImageRegistryHandler{store: s, encKey: encKey, verifyConnection: verifyRegistryConnection}
}

func (h *ImageRegistryHandler) List(c *gin.Context) {
	projectID, err := optionalID(c.Query("project_id"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	registries, err := h.store.ListImageRegistries(projectID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, registries)
}

func (h *ImageRegistryHandler) Create(c *gin.Context) {
	var req imageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "镜像仓库定义无效")
		return
	}
	registry, err := h.registryFromRequest(req, nil)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	registry.CreatedBy = apiShared.UserID(c)
	if err := h.store.CreateImageRegistry(registry, req.ProjectIDs); err != nil {
		apiShared.Conflict(c, "镜像仓库名称、地址或项目授权无效")
		return
	}
	registry.CredentialConfigured = registryservice.CredentialConfigured(registry.Credential)
	model.Success(c, registry)
}

func (h *ImageRegistryHandler) Update(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像仓库 ID 无效")
		return
	}
	current, err := h.store.GetImageRegistry(id)
	if err != nil {
		apiShared.NotFound(c, "镜像仓库不存在")
		return
	}
	if current.ManagedRegistryID != nil {
		apiShared.Conflict(c, "该镜像仓库由受管制品库维护，请在交付中心修改")
		return
	}
	var req imageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "镜像仓库定义无效")
		return
	}
	registry, err := h.registryFromRequest(req, current)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.store.UpdateImageRegistry(registry, req.ProjectIDs); err != nil {
		apiShared.Conflict(c, "镜像仓库名称、地址或项目授权无效")
		return
	}
	registry.CredentialConfigured = registryservice.CredentialConfigured(registry.Credential)
	model.Success(c, registry)
}

func (h *ImageRegistryHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像仓库 ID 无效")
		return
	}
	registry, err := h.store.GetImageRegistry(id)
	if err != nil {
		apiShared.NotFound(c, "镜像仓库不存在")
		return
	}
	if registry.ManagedRegistryID != nil {
		apiShared.Conflict(c, "该镜像仓库由受管制品库维护，不能单独删除")
		return
	}
	count, err := h.store.CountImageRegistryReleases(id)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if count > 0 {
		apiShared.Conflict(c, "镜像仓库已被发布记录引用，无法删除")
		return
	}
	if err := h.store.DeleteImageRegistry(id); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *ImageRegistryHandler) Verify(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "镜像仓库 ID 无效")
		return
	}
	registry, err := h.store.GetImageRegistry(id)
	if err != nil {
		apiShared.NotFound(c, "镜像仓库不存在")
		return
	}
	status, detail := "succeeded", ""
	if !registry.Enabled {
		status, detail = "failed", "镜像仓库已停用"
	} else if err := h.verifyConnection(c.Request.Context(), registry, h.encKey); err != nil {
		status, detail = "failed", verificationDetail(err)
	}
	if err := h.store.UpdateImageRegistryVerification(registry.ID, status, detail, time.Now()); err != nil {
		apiShared.DBError(c, "保存检测结果失败")
		return
	}
	registry, err = h.store.GetImageRegistry(id)
	if err != nil {
		apiShared.DBError(c, "读取检测结果失败")
		return
	}
	model.Success(c, registry)
}

func verifyRegistryConnection(ctx context.Context, registry *model.ImageRegistry, encKey []byte) error {
	ref, err := registryservice.VerificationImageReference(registry.Endpoint, registry.VerificationImage, "验证镜像必须属于当前镜像仓库地址")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	options := []remote.Option{remote.WithContext(ctx)}
	if registry.AuthType != registryservice.AuthTypeAnonymous {
		credential, err := registryservice.DecryptCredential(encKey, registry.Credential)
		if err != nil {
			return fmt.Errorf("读取镜像仓库凭据失败")
		}
		authConfig := authn.AuthConfig{}
		if registry.AuthType == registryservice.AuthTypeToken {
			authConfig.RegistryToken = credential
		} else {
			authConfig.Username = registry.Username
			authConfig.Password = credential
		}
		options = append(options, remote.WithAuth(authn.FromConfig(authConfig)))
	} else {
		options = append(options, remote.WithAuth(authn.Anonymous))
	}
	_, err = remote.Head(ref, options...)
	if err != nil {
		return fmt.Errorf("无法访问验证镜像: %w", err)
	}
	return nil
}

func verificationDetail(err error) string {
	detail := strings.TrimSpace(err.Error())
	if len(detail) > 480 {
		return detail[:480]
	}
	return detail
}

func (h *ImageRegistryHandler) registryFromRequest(req imageRegistryRequest, current *model.ImageRegistry) (*model.ImageRegistry, error) {
	endpoint, err := registryservice.NormalizeImageRegistryEndpoint(req.Endpoint)
	if err != nil {
		return nil, err
	}
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		authType = registryservice.AuthTypeAnonymous
	}
	if authType != registryservice.AuthTypeAnonymous && authType != registryservice.AuthTypeBasic && authType != registryservice.AuthTypeToken {
		return nil, errInvalid("认证方式仅支持匿名、账号密码或 Token")
	}
	verificationImage := strings.TrimSpace(req.VerificationImage)
	if _, err := registryservice.VerificationImageReference(endpoint, verificationImage, "验证镜像必须属于当前镜像仓库地址"); err != nil {
		return nil, err
	}
	registry := &model.ImageRegistry{Name: strings.TrimSpace(req.Name), Endpoint: endpoint, VerificationImage: verificationImage, AuthType: authType, Username: strings.TrimSpace(req.Username), Enabled: true}
	if registry.Name == "" {
		return nil, errInvalid("镜像仓库名称必填")
	}
	if current != nil {
		registry.ID = current.ID
		registry.CreatedBy = current.CreatedBy
		registry.CreatedAt = current.CreatedAt
		registry.Credential = current.Credential
		if req.Enabled != nil {
			registry.Enabled = *req.Enabled
		} else {
			registry.Enabled = current.Enabled
		}
	}
	if req.Credential != nil {
		credential := strings.TrimSpace(*req.Credential)
		if credential == "" && authType != registryservice.AuthTypeAnonymous {
			return nil, errInvalid("镜像仓库凭据必填")
		}
		if credential != "" {
			encrypted, err := registryservice.EncryptCredential(h.encKey, credential)
			if err != nil {
				return nil, errInvalid("镜像仓库凭据加密失败")
			}
			registry.Credential = encrypted
		}
	}
	if authType == registryservice.AuthTypeAnonymous {
		registry.Username = ""
		registry.Credential = ""
	} else if registry.Credential == "" {
		return nil, errInvalid("镜像仓库凭据必填")
	} else if authType == registryservice.AuthTypeBasic && registry.Username == "" {
		return nil, errInvalid("账号密码认证需要填写账号")
	}
	return registry, nil
}

type invalidRegistryRequest string

func (e invalidRegistryRequest) Error() string { return string(e) }
func errInvalid(message string) error          { return invalidRegistryRequest(message) }

func optionalID(value string) (uint, error) {
	if value == "" {
		return 0, nil
	}
	return apiShared.ParseID(value)
}
