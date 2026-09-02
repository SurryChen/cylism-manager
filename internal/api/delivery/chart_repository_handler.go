package delivery

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ChartRepositoryHandler struct {
	store  repository.ChartRepositoryStore
	client *http.Client
}
type chartRepositoryRequest struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	ChartName    string `json:"chart_name"`
	ChartVersion string `json:"chart_version"`
	Enabled      *bool  `json:"enabled"`
}

func NewChartRepositoryHandler(s repository.ChartRepositoryStore) *ChartRepositoryHandler {
	return &ChartRepositoryHandler{store: s, client: &http.Client{Timeout: 10 * time.Second}}
}
func (h *ChartRepositoryHandler) List(c *gin.Context) {
	items, e := h.store.ListChartRepositories()
	if e != nil {
		apiShared.Error(c, 500, model.CodeDBError, "读取 Chart 仓库失败")
		return
	}
	model.Success(c, items)
}
func (h *ChartRepositoryHandler) Create(c *gin.Context) {
	var r chartRepositoryRequest
	if c.ShouldBindJSON(&r) != nil {
		apiShared.Error(c, 400, model.CodeBadRequest, "Chart 仓库定义无效")
		return
	}
	item, e := chartRepositoryFromRequest(r, nil)
	if e != nil {
		apiShared.Error(c, 400, model.CodeValidationFail, e.Error())
		return
	}
	item.CreatedBy = apiShared.UserID(c)
	if e = h.store.CreateChartRepository(item); e != nil {
		apiShared.Error(c, 409, model.CodeConflict, "Chart 仓库名称或地址已存在")
		return
	}
	model.Success(c, item)
}
func (h *ChartRepositoryHandler) Update(c *gin.Context) {
	id, e := apiShared.ParseID(c.Param("id"))
	if e != nil {
		apiShared.Error(c, 400, model.CodeBadRequest, "Chart 仓库 ID 无效")
		return
	}
	old, e := h.store.GetChartRepository(id)
	if e != nil {
		apiShared.Error(c, 404, model.CodeNotFound, "Chart 仓库不存在")
		return
	}
	var r chartRepositoryRequest
	if c.ShouldBindJSON(&r) != nil {
		apiShared.Error(c, 400, model.CodeBadRequest, "Chart 仓库定义无效")
		return
	}
	item, e := chartRepositoryFromRequest(r, old)
	if e != nil {
		apiShared.Error(c, 400, model.CodeValidationFail, e.Error())
		return
	}
	if e = h.store.UpdateChartRepository(item); e != nil {
		apiShared.Error(c, 409, model.CodeConflict, "Chart 仓库名称或地址已存在")
		return
	}
	model.Success(c, item)
}
func (h *ChartRepositoryHandler) Delete(c *gin.Context) {
	id, e := apiShared.ParseID(c.Param("id"))
	if e != nil {
		apiShared.Error(c, 400, model.CodeBadRequest, "Chart 仓库 ID 无效")
		return
	}
	if e = h.store.DeleteChartRepository(id); e != nil {
		apiShared.Error(c, 500, model.CodeDBError, "删除 Chart 仓库失败")
		return
	}
	model.Success(c, gin.H{"id": id})
}
func (h *ChartRepositoryHandler) Verify(c *gin.Context) {
	id, e := apiShared.ParseID(c.Param("id"))
	if e != nil {
		apiShared.Error(c, 400, model.CodeBadRequest, "Chart 仓库 ID 无效")
		return
	}
	item, e := h.store.GetChartRepository(id)
	if e != nil {
		apiShared.Error(c, 404, model.CodeNotFound, "Chart 仓库不存在")
		return
	}
	status, detail := "succeeded", ""
	resp, e := h.client.Get(strings.TrimRight(item.Endpoint, "/") + "/index.yaml")
	if e != nil {
		status, detail = "failed", e.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status, detail = "failed", resp.Status
		}
	}
	now := time.Now()
	item.LastVerifiedAt = &now
	item.LastVerifyStatus = status
	item.LastVerifyError = detail
	_ = h.store.UpdateChartRepository(item)
	model.Success(c, item)
}
func chartRepositoryFromRequest(r chartRepositoryRequest, old *model.ChartRepository) (*model.ChartRepository, error) {
	u, e := url.ParseRequestURI(strings.TrimSpace(r.Endpoint))
	if e != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errInvalid("Chart 仓库地址必须是 HTTPS URL")
	}
	item := &model.ChartRepository{Name: strings.TrimSpace(r.Name), Endpoint: strings.TrimRight(strings.TrimSpace(r.Endpoint), "/"), ChartName: strings.TrimSpace(r.ChartName), ChartVersion: strings.TrimSpace(r.ChartVersion), Enabled: true}
	if item.Name == "" || item.ChartName == "" || item.ChartVersion == "" {
		return nil, errInvalid("名称、Chart 名称和版本必填")
	}
	if old != nil {
		item.ID = old.ID
		item.CreatedAt = old.CreatedAt
		item.CreatedBy = old.CreatedBy
		item.LastVerifiedAt = old.LastVerifiedAt
		item.LastVerifyStatus = old.LastVerifyStatus
		item.LastVerifyError = old.LastVerifyError
	}
	if r.Enabled != nil {
		item.Enabled = *r.Enabled
	}
	return item, nil
}
