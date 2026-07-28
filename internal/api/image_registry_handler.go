package api

import (
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

const (
	registryAuthAnonymous = "anonymous"
	registryAuthBasic     = "basic"
	registryAuthToken     = "token"
)

type ImageRegistryHandler struct {
	store  *store.Store
	encKey []byte
}

type imageRegistryRequest struct {
	Name       string  `json:"name"`
	Endpoint   string  `json:"endpoint"`
	AuthType   string  `json:"auth_type"`
	Username   string  `json:"username"`
	Credential *string `json:"credential"`
	Enabled    *bool   `json:"enabled"`
	ProjectIDs []uint  `json:"project_ids"`
}

func NewImageRegistryHandler(s *store.Store, encKey []byte) *ImageRegistryHandler {
	return &ImageRegistryHandler{store: s, encKey: encKey}
}

func (h *ImageRegistryHandler) List(c *gin.Context) {
	projectID, err := optionalID(c.Query("project_id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "项目 ID 无效")
		return
	}
	registries, err := h.store.ListImageRegistries(projectID)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, registries)
}

func (h *ImageRegistryHandler) Create(c *gin.Context) {
	var req imageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像仓库定义无效")
		return
	}
	registry, err := h.registryFromRequest(req, nil)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	registry.CreatedBy = getUserID(c)
	if err := h.store.CreateImageRegistry(registry, req.ProjectIDs); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像仓库名称、地址或项目授权无效")
		return
	}
	registry.CredentialConfigured = registry.Credential != ""
	model.Success(c, registry)
}

func (h *ImageRegistryHandler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像仓库 ID 无效")
		return
	}
	current, err := h.store.GetImageRegistry(id)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像仓库不存在")
		return
	}
	var req imageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像仓库定义无效")
		return
	}
	registry, err := h.registryFromRequest(req, current)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	if err := h.store.UpdateImageRegistry(registry, req.ProjectIDs); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像仓库名称、地址或项目授权无效")
		return
	}
	registry.CredentialConfigured = registry.Credential != ""
	model.Success(c, registry)
}

func (h *ImageRegistryHandler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "镜像仓库 ID 无效")
		return
	}
	if _, err := h.store.GetImageRegistry(id); err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "镜像仓库不存在")
		return
	}
	count, err := h.store.CountImageRegistryReleases(id)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	if count > 0 {
		model.Error(c, http.StatusConflict, model.CodeConflict, "镜像仓库已被发布记录引用，无法删除")
		return
	}
	if err := h.store.DeleteImageRegistry(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeDBError, err.Error())
		return
	}
	model.Success(c, gin.H{"id": id})
}

func (h *ImageRegistryHandler) registryFromRequest(req imageRegistryRequest, current *model.ImageRegistry) (*model.ImageRegistry, error) {
	endpoint, err := normalizeRegistryEndpoint(req.Endpoint)
	if err != nil {
		return nil, err
	}
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		authType = registryAuthAnonymous
	}
	if authType != registryAuthAnonymous && authType != registryAuthBasic && authType != registryAuthToken {
		return nil, errInvalid("认证方式仅支持匿名、账号密码或 Token")
	}
	registry := &model.ImageRegistry{Name: strings.TrimSpace(req.Name), Endpoint: endpoint, AuthType: authType, Username: strings.TrimSpace(req.Username), Enabled: true}
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
		if credential == "" && authType != registryAuthAnonymous {
			return nil, errInvalid("镜像仓库凭据必填")
		}
		if credential != "" {
			encrypted, err := crypto.Encrypt(h.encKey, credential)
			if err != nil {
				return nil, errInvalid("镜像仓库凭据加密失败")
			}
			registry.Credential = encrypted
		}
	}
	if authType == registryAuthAnonymous {
		registry.Username = ""
		registry.Credential = ""
	} else if registry.Credential == "" {
		return nil, errInvalid("镜像仓库凭据必填")
	} else if authType == registryAuthBasic && registry.Username == "" {
		return nil, errInvalid("账号密码认证需要填写账号")
	}
	if len(req.ProjectIDs) == 0 {
		return nil, errInvalid("至少授权一个项目")
	}
	return registry, nil
}

type invalidRegistryRequest string

func (e invalidRegistryRequest) Error() string { return string(e) }
func errInvalid(message string) error          { return invalidRegistryRequest(message) }

func normalizeRegistryEndpoint(value string) (string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(value), "/")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if endpoint == "" || strings.ContainsAny(endpoint, " /?#@") || strings.Contains(endpoint, "://") {
		return "", errInvalid("镜像仓库地址格式无效")
	}
	return endpoint, nil
}

func optionalID(value string) (uint, error) {
	if value == "" {
		return 0, nil
	}
	return parseID(value)
}
