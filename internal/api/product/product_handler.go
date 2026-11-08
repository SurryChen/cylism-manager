package productapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Repository interface {
	ListProducts(uint) ([]model.Product, error)
	GetProduct(uint, uint) (*model.Product, error)
	SaveProduct(*model.Product, []uint, []uint) error
	ProductResourceOptions(uint) ([]model.Project, []model.Application, error)
}

type Handler struct{ repository Repository }

func NewHandler(repository Repository) *Handler { return &Handler{repository: repository} }

type productInput struct {
	Name           string                 `json:"name"`
	Summary        string                 `json:"summary"`
	IconURL        string                 `json:"icon_url"`
	Identifier     string                 `json:"identifier"`
	Stage          string                 `json:"stage"`
	Links          []model.ProductLink    `json:"links"`
	NextAction     string                 `json:"next_action"`
	NextDueDate    string                 `json:"next_due_date"`
	PublicBio      string                 `json:"public_bio"`
	PublicURL      string                 `json:"public_url"`
	PublicShowcase bool                   `json:"public_showcase"`
	Channels       []model.ProductChannel `json:"channels"`
	ProjectIDs     []uint                 `json:"project_ids"`
	ApplicationIDs []uint                 `json:"application_ids"`
}

func (h *Handler) List(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	products, err := h.repository.ListProducts(shared.UserID(c))
	if err != nil {
		shared.DBError(c, "读取产品失败")
		return
	}
	shared.Success(c, products)
}

func (h *Handler) Get(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	id, err := shared.ParsePositiveID(c.Param("id"))
	if err != nil {
		shared.BadRequest(c, "产品 ID 无效")
		return
	}
	product, err := h.repository.GetProduct(shared.UserID(c), id)
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, product)
}

func (h *Handler) Options(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	projects, apps, err := h.repository.ProductResourceOptions(shared.UserID(c))
	if err != nil {
		shared.DBError(c, "读取可关联资源失败")
		return
	}
	// Explicit DTOs exclude deployment specifications, credentials and registry data.
	type option struct {
		ID            uint   `json:"id"`
		Name          string `json:"name"`
		ProjectID     uint   `json:"project_id,omitempty"`
		EnvironmentID uint   `json:"environment_id,omitempty"`
	}
	projectOptions, appOptions := []option{}, []option{}
	for _, p := range projects {
		projectOptions = append(projectOptions, option{ID: p.ID, Name: p.Name})
	}
	for _, a := range apps {
		appOptions = append(appOptions, option{ID: a.ID, Name: a.Name, ProjectID: a.ProjectID, EnvironmentID: a.EnvironmentID})
	}
	shared.Success(c, gin.H{"projects": projectOptions, "applications": appOptions})
}

func (h *Handler) Create(c *gin.Context) { h.save(c, false) }
func (h *Handler) Update(c *gin.Context) { h.save(c, true) }

func (h *Handler) save(c *gin.Context, update bool) {
	if !authenticated(c) {
		return
	}
	var id uint
	if update {
		var err error
		id, err = shared.ParsePositiveID(c.Param("id"))
		if err != nil {
			shared.BadRequest(c, "产品 ID 无效")
			return
		}
	}
	var input productInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		shared.BadRequest(c, "产品资料格式无效或内容过大")
		return
	}
	product, err := input.product()
	if err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	product.ID, product.OwnerID = id, shared.UserID(c)
	if err := h.repository.SaveProduct(product, input.ProjectIDs, input.ApplicationIDs); err != nil {
		respondError(c, err)
		return
	}
	// Save returns the aggregate directly; a later read refreshes related resource names.
	shared.Success(c, product)
}

func authenticated(c *gin.Context) bool {
	if shared.UserID(c) != 0 {
		return true
	}
	shared.Unauthorized(c, "请先登录")
	return false
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		shared.NotFound(c, "产品不存在或无权访问")
	case errors.Is(err, model.ErrProductResourceAccess):
		shared.Error(c, http.StatusForbidden, shared.CodeForbidden, err.Error())
	default:
		shared.DBError(c, "保存或读取产品失败")
	}
}

func (input *productInput) product() (*model.Product, error) {
	input.Name, input.Stage = strings.TrimSpace(input.Name), strings.TrimSpace(input.Stage)
	if input.Name == "" {
		return nil, errors.New("产品名称必填")
	}
	if input.Stage == "" {
		input.Stage = model.ProductStageIdea
	}
	if !oneOf(input.Stage, "idea", "developing", "operating", "paused", "archived") {
		return nil, errors.New("产品阶段无效")
	}
	if len(input.Links) > 30 || len(input.Channels) > 30 || len(input.ProjectIDs) > 100 || len(input.ApplicationIDs) > 100 {
		return nil, errors.New("链接、渠道或关联资源数量超出限制")
	}
	for name, field := range map[string]struct {
		value string
		max   int
	}{
		"名称": {input.Name, 128}, "简介": {input.Summary, 1024}, "标识": {input.Identifier, 32}, "下一步行动": {input.NextAction, 512}, "公开简介": {input.PublicBio, 1024},
	} {
		if utf8.RuneCountInString(field.value) > field.max {
			return nil, fmt.Errorf("%s不能超过 %d 字", name, field.max)
		}
	}
	for _, value := range []string{input.IconURL, input.PublicURL} {
		if err := validateURL(value); err != nil {
			return nil, err
		}
	}
	for _, link := range input.Links {
		if !oneOf(link.Kind, "repository", "design", "requirements", "documentation", "other") || strings.TrimSpace(link.URL) == "" || utf8.RuneCountInString(link.Label) > 128 {
			return nil, errors.New("资料链接类型、名称或地址无效")
		}
		if err := validateURL(link.URL); err != nil {
			return nil, err
		}
	}
	var due *time.Time
	if input.NextDueDate != "" {
		parsed, err := time.Parse("2006-01-02", input.NextDueDate)
		if err != nil || strings.TrimSpace(input.NextAction) == "" {
			return nil, errors.New("截止日期需使用有效日期，并填写下一步行动")
		}
		due = &parsed
	}
	for i := range input.Channels {
		ch := &input.Channels[i]
		if !oneOf(ch.Platform, "wechat_mini_program", "douyin_mini_program", "mini_game", "google_play", "app_store", "web", "other") {
			return nil, errors.New("发行平台无效")
		}
		if ch.PublishStatus == "" {
			ch.PublishStatus = "draft"
		}
		if !oneOf(ch.PublishStatus, "draft", "preparing", "ready", "in_review", "approved", "published", "rejected", "blocked", "paused") {
			return nil, errors.New("发行状态无效")
		}
		for _, value := range []string{ch.AdminURL, ch.PublicURL} {
			if err := validateURL(value); err != nil {
				return nil, err
			}
		}
		if utf8.RuneCountInString(ch.Name) > 128 || utf8.RuneCountInString(ch.AppIdentifier) > 256 || utf8.RuneCountInString(ch.OnlineVersion) > 128 || utf8.RuneCountInString(ch.PendingVersion) > 128 || utf8.RuneCountInString(ch.Notes) > 1024 {
			return nil, errors.New("渠道字段长度超出限制")
		}
		ch.ProductID, ch.CreatedAt, ch.UpdatedAt = 0, time.Time{}, time.Time{}
	}
	return &model.Product{Name: input.Name, Summary: input.Summary, IconURL: input.IconURL, Identifier: input.Identifier, Stage: input.Stage, Links: input.Links, NextAction: strings.TrimSpace(input.NextAction), NextDueDate: due, PublicBio: input.PublicBio, PublicURL: input.PublicURL, PublicShowcase: input.PublicShowcase, Channels: input.Channels}, nil
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func validateURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || len(value) > 2048 || strings.TrimSpace(value) != value || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("链接需为不含账号密码的 HTTP 或 HTTPS 地址")
	}
	return nil
}
