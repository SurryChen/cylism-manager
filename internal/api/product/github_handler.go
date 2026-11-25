package productapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/githubapp"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GitHubRepository interface {
	GitHubConnection(uint) (model.GitHubConnection, error)
	StartGitHubRegistration(*model.GitHubRegistration) error
	ClaimGitHubRegistration(string, string, time.Time) (model.GitHubRegistration, error)
	FinishGitHubRegistration(model.GitHubRegistration, *model.GitHubConnection) error
	DeleteGitHubConnection(uint) error
	ListProductGitHubRepositories(uint, uint) ([]model.ProductGitHubRepository, error)
	ProductGitHubRepository(uint, uint, uint) (model.ProductGitHubRepository, error)
	SaveProductGitHubRepository(uint, *model.ProductGitHubRepository) error
	DeleteProductGitHubRepository(uint, uint, uint) error
	ProductVersionGitHubSources(uint, uint, uint) (model.ProductVersion, error)
}

type githubIntegration struct {
	repository GitHubRepository
	client     githubClient
	key        []byte
	publicURL  string
}

type githubClient interface {
	ConvertManifest(context.Context, string) (githubapp.AppRegistration, error)
	Installations(context.Context, string, int) ([]githubapp.Installation, bool, error)
	Repositories(context.Context, string, int64, int) ([]githubapp.Repository, bool, error)
	OpenRepository(context.Context, string, int64, int64) (*githubapp.RepositorySession, error)
}

func (h *Handler) WithGitHub(repository GitHubRepository, key []byte, publicURL string) *Handler {
	h.github = &githubIntegration{repository: repository, key: key, publicURL: publicURL, client: githubapp.NewClient()}
	return h
}

func (h *Handler) githubAvailable(c *gin.Context) bool {
	if !authenticated(c) {
		return false
	}
	if h.github == nil {
		shared.NotFound(c, "GitHub 集成不可用")
		return false
	}
	c.Header("Cache-Control", "no-store")
	return true
}

func (h *Handler) GitHubConnection(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	connection, err := h.github.repository.GitHubConnection(shared.UserID(c))
	if errors.Is(err, model.ErrGitHubNotConfigured) {
		shared.Success(c, nil)
		return
	}
	if err != nil {
		shared.DBError(c, "读取 GitHub App 配置失败")
		return
	}
	shared.Success(c, connection)
}

func (h *Handler) DeleteGitHubConnection(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	if err := h.github.repository.DeleteGitHubConnection(shared.UserID(c)); err != nil {
		shared.DBError(c, "解除 GitHub App 连接失败")
		return
	}
	shared.Success(c, nil)
}

func (g *githubIntegration) token(ownerID uint) (model.GitHubConnection, string, error) {
	connection, err := g.repository.GitHubConnection(ownerID)
	if err != nil {
		return connection, "", err
	}
	key, err := security.Decrypt(g.key, connection.EncryptedKey)
	if err != nil {
		return connection, "", errors.New("无法解密 GitHub App 私钥")
	}
	token, err := githubapp.AppToken(connection.AppID, key)
	return connection, token, err
}

func githubError(c *gin.Context, err error) {
	var remote githubapp.HTTPError
	var request githubapp.RequestError
	switch {
	case errors.Is(err, model.ErrGitHubNotConfigured):
		shared.Error(c, http.StatusConflict, shared.CodeConflict, err.Error())
	case errors.As(err, &remote), errors.As(err, &request):
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
	default:
		respondError(c, err)
	}
}

func githubPage(c *gin.Context) (int, bool) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000 {
		shared.BadRequest(c, "GitHub 页码无效")
		return 0, false
	}
	return page, true
}

func (h *Handler) GitHubInstallations(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	page, ok := githubPage(c)
	if !ok {
		return
	}
	_, token, err := h.github.token(shared.UserID(c))
	if err != nil {
		githubError(c, err)
		return
	}
	items, more, err := h.github.client.Installations(c.Request.Context(), token, page)
	if err != nil {
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
		return
	}
	shared.Success(c, gin.H{"items": items, "has_more": more})
}

func (h *Handler) GitHubInstallationRepositories(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	installationID, err := strconv.ParseInt(c.Param("installationID"), 10, 64)
	if err != nil || installationID <= 0 {
		shared.BadRequest(c, "GitHub 安装 ID 无效")
		return
	}
	page, ok := githubPage(c)
	if !ok {
		return
	}
	_, token, err := h.github.token(shared.UserID(c))
	if err != nil {
		githubError(c, err)
		return
	}
	items, more, err := h.github.client.Repositories(c.Request.Context(), token, installationID, page)
	if err != nil {
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
		return
	}
	shared.Success(c, gin.H{"items": items, "has_more": more})
}

func (h *Handler) GitHubProductRepositories(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	productID, err := shared.ParsePositiveID(c.Param("id"))
	if err != nil {
		shared.BadRequest(c, "产品 ID 无效")
		return
	}
	items, err := h.github.repository.ListProductGitHubRepositories(shared.UserID(c), productID)
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, items)
}

func (h *Handler) BindGitHubRepository(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	productID, err := shared.ParsePositiveID(c.Param("id"))
	if err != nil {
		shared.BadRequest(c, "产品 ID 无效")
		return
	}
	ownerID := shared.UserID(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var input struct {
		ReleaseUnitID  uint   `json:"release_unit_id"`
		Component      string `json:"component"`
		InstallationID int64  `json:"installation_id"`
		RepositoryID   int64  `json:"repository_id"`
	}
	if c.ShouldBindJSON(&input) != nil || input.InstallationID <= 0 || input.RepositoryID <= 0 {
		shared.BadRequest(c, "请选择 GitHub 安装和仓库")
		return
	}
	input.Component = strings.TrimSpace(input.Component)
	if err := lifecycleText(input.Component, 128, true); err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	product, err := h.repository.GetProduct(ownerID, productID)
	if err != nil {
		respondError(c, err)
		return
	}
	if input.ReleaseUnitID != 0 && !slices.ContainsFunc(product.ReleaseUnits, func(unit model.ProductReleaseUnit) bool { return unit.ID == input.ReleaseUnitID }) {
		respondError(c, model.ErrProductResourceAccess)
		return
	}
	connection, token, err := h.github.token(ownerID)
	if err != nil {
		githubError(c, err)
		return
	}
	session, err := h.github.client.OpenRepository(c.Request.Context(), token, input.InstallationID, input.RepositoryID)
	if err != nil {
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
		return
	}
	binding := model.ProductGitHubRepository{
		ProductID: productID, ReleaseUnitID: input.ReleaseUnitID, Component: input.Component, AppID: connection.AppID,
		InstallationID: input.InstallationID, RepositoryID: session.Repository.ID, Repository: session.Repository.FullName,
	}
	if err := h.github.repository.SaveProductGitHubRepository(ownerID, &binding); err != nil {
		githubError(c, err)
		return
	}
	shared.Success(c, binding)
}

func githubBindingIDs(c *gin.Context) (uint, uint, bool) {
	productID, err := shared.ParsePositiveID(c.Param("id"))
	if err != nil {
		return 0, 0, false
	}
	bindingID, err := shared.ParsePositiveID(c.Param("bindingID"))
	return productID, bindingID, err == nil
}

func (h *Handler) UnbindGitHubRepository(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	productID, bindingID, ok := githubBindingIDs(c)
	if !ok {
		shared.BadRequest(c, "产品或仓库绑定 ID 无效")
		return
	}
	if err := h.github.repository.DeleteProductGitHubRepository(shared.UserID(c), productID, bindingID); err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, nil)
}

func (h *Handler) GitHubRevisions(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	productID, bindingID, ok := githubBindingIDs(c)
	if !ok {
		shared.BadRequest(c, "产品或仓库绑定 ID 无效")
		return
	}
	page, ok := githubPage(c)
	if !ok {
		return
	}
	kind := c.DefaultQuery("kind", "tags")
	if !oneOf(kind, "tags", "commits") {
		shared.BadRequest(c, "请选择 Tag 或提交")
		return
	}
	ownerID := shared.UserID(c)
	binding, err := h.github.repository.ProductGitHubRepository(ownerID, productID, bindingID)
	if err != nil {
		respondError(c, err)
		return
	}
	connection, token, err := h.github.token(ownerID)
	if err != nil {
		githubError(c, err)
		return
	}
	if binding.AppID != connection.AppID {
		shared.Error(c, http.StatusConflict, shared.CodeConflict, "该绑定属于之前的 GitHub App，请重新绑定仓库")
		return
	}
	session, err := h.github.client.OpenRepository(c.Request.Context(), token, binding.InstallationID, binding.RepositoryID)
	if err != nil {
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
		return
	}
	items, more, err := session.Revisions(c.Request.Context(), kind, page)
	if err != nil {
		shared.Error(c, http.StatusBadGateway, shared.CodeBadRequest, err.Error())
		return
	}
	shared.Success(c, gin.H{"items": items, "has_more": more})
}

type githubSourceError string

func (e githubSourceError) Error() string { return string(e) }

func respondGitHubSourceError(c *gin.Context, err error) {
	var invalid githubSourceError
	var revision githubapp.InvalidRevision
	switch {
	case errors.As(err, &invalid), errors.As(err, &revision):
		shared.ValidationError(c, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		shared.NotFound(c, "版本或 GitHub 仓库绑定不存在")
	default:
		githubError(c, err)
	}
}

type githubSourceInput struct {
	BindingID uint   `json:"binding_id"`
	Tag       string `json:"tag"`
	SHA       string `json:"sha"`
}

func resolveGitHubSources(ctx context.Context, g *githubIntegration, ownerID uint, version *model.ProductVersion, inputs []githubSourceInput) error {
	if inputs == nil {
		return nil
	}
	version.GitHubSources = make([]model.ProductGitHubSource, 0, len(inputs))
	if len(inputs) == 0 {
		return nil
	}
	if g == nil {
		return model.ErrGitHubNotConfigured
	}
	if len(inputs) > 20 {
		return githubSourceError("单个版本最多可关联 20 个组件来源")
	}
	var current model.ProductVersion
	if version.ID != 0 {
		var err error
		current, err = g.repository.ProductVersionGitHubSources(ownerID, version.ProductID, version.ID)
		if err != nil {
			return err
		}
		if version.Revision != current.Revision {
			return model.ErrProductLifecycleConflict
		}
		if version.KeepReleaseUnit {
			version.ReleaseUnitID = current.ReleaseUnitID
		}
	}
	var connection model.GitHubConnection
	var token string
	components := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		if input.BindingID == 0 || len(input.Tag) > 512 || len(input.SHA) > 40 {
			return githubSourceError("GitHub 版本来源无效")
		}
		index := slices.IndexFunc(current.GitHubSources, func(source model.ProductGitHubSource) bool {
			return current.ReleaseUnitID == version.ReleaseUnitID && source.BindingID == input.BindingID && source.Tag == input.Tag && source.SHA == input.SHA
		})
		var source model.ProductGitHubSource
		if index >= 0 {
			source = current.GitHubSources[index]
		} else {
			binding, err := g.repository.ProductGitHubRepository(ownerID, version.ProductID, input.BindingID)
			if err != nil {
				return err
			}
			if binding.ReleaseUnitID != version.ReleaseUnitID {
				return githubSourceError("请选择当前发布端绑定的仓库")
			}
			if token == "" {
				connection, token, err = g.token(ownerID)
				if err != nil {
					return err
				}
			}
			if binding.AppID != connection.AppID {
				return githubSourceError("该绑定属于之前的 GitHub App，请重新绑定仓库")
			}
			session, err := g.client.OpenRepository(ctx, token, binding.InstallationID, binding.RepositoryID)
			if err != nil {
				return err
			}
			revision, err := session.Resolve(ctx, input.Tag, input.SHA)
			if err != nil {
				return err
			}
			source = model.ProductGitHubSource{
				BindingID: binding.ID, Component: binding.Component, Repository: session.Repository.FullName,
				RepositoryID: binding.RepositoryID, Tag: revision.Tag, SHA: revision.SHA,
			}
		}
		if components[source.Component] {
			return githubSourceError("每个组件只能关联一个 GitHub 来源")
		}
		components[source.Component] = true
		version.GitHubSources = append(version.GitHubSources, source)
	}
	return nil
}
