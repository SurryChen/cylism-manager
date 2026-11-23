package productapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type LifecycleRepository interface {
	ListProductTasks(uint, uint, model.ProductLifecycleFilter) ([]model.ProductTask, int64, error)
	ListProductVersions(uint, uint, model.ProductLifecycleFilter) ([]model.ProductVersion, int64, error)
	ListProductDistributions(uint, uint, model.ProductLifecycleFilter) ([]model.ProductDistribution, int64, error)
	ListProductDistributionEvents(uint, uint, uint, model.ProductLifecycleFilter) ([]model.ProductDistributionEvent, int64, error)
	ProductDeploymentPage(uint, uint, model.ProductLifecycleFilter) ([]model.ProductDeploymentOption, int64, error)
	ProductVersionOptions(uint, uint) ([]model.ProductVersion, error)
	SaveProductTask(uint, *model.ProductTask) error
	SaveProductVersion(uint, *model.ProductVersion) error
	SaveProductDistribution(uint, *model.ProductDistribution) error
	ProductWorkbench(uint) (*model.ProductWorkbench, error)
	ImportProductNextAction(uint, uint) (*model.ProductTask, error)
}

func lifecycleID(c *gin.Context) (uint, bool) {
	if !authenticated(c) {
		return 0, false
	}
	id, err := shared.ParsePositiveID(c.Param("id"))
	if err != nil {
		shared.BadRequest(c, "产品 ID 无效")
		return 0, false
	}
	return id, true
}

func (h *Handler) Workbench(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	result, err := h.repository.ProductWorkbench(shared.UserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, result)
}

func (h *Handler) LifecycleOptions(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	rows, err := h.repository.ProductVersionOptions(shared.UserID(c), id)
	if err != nil {
		respondError(c, err)
		return
	}
	type option struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	options := []option{}
	for _, row := range rows {
		options = append(options, option{row.ID, row.Name, row.Status})
	}
	shared.Success(c, gin.H{"versions": options})
}

func (h *Handler) ListLifecycle(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	limit, offset := shared.LimitOffset(c, 10, 100)
	f := model.ProductLifecycleFilter{Status: c.Query("status"), Keyword: strings.TrimSpace(c.Query("keyword")), Limit: limit, Offset: offset}
	if raw := c.Query("version_id"); raw != "" {
		var err error
		f.VersionID, err = shared.ParsePositiveID(raw)
		if err != nil {
			shared.BadRequest(c, "版本 ID 无效")
			return
		}
	}
	kind := c.Param("kind")
	if kind == "versions" {
		f.VersionID = 0
	}
	var rows interface{}
	var count int64
	var err error
	switch kind {
	case "tasks":
		rows, count, err = h.repository.ListProductTasks(shared.UserID(c), id, f)
	case "versions":
		rows, count, err = h.repository.ListProductVersions(shared.UserID(c), id, f)
	case "distributions":
		rows, count, err = h.repository.ListProductDistributions(shared.UserID(c), id, f)
	case "deployments":
		rows, count, err = h.repository.ProductDeploymentPage(shared.UserID(c), id, f)
	default:
		shared.NotFound(c, "列表不存在")
		return
	}
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, gin.H{"items": rows, "total": count, "limit": limit, "offset": offset})
}

func lifecycleDate(value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return fmt.Errorf("请填写有效日期（YYYY-MM-DD）")
	}
	return nil
}
func lifecycleText(value string, max int, required bool) error {
	if (required && strings.TrimSpace(value) == "") || utf8.RuneCountInString(value) > max {
		return fmt.Errorf("必填内容不能为空，且字段长度不能超过 %d 字", max)
	}
	return nil
}
func (h *Handler) SaveLifecycle(c *gin.Context) {
	productID, ok := lifecycleID(c)
	if !ok {
		return
	}
	var id uint
	if raw := c.Param("recordID"); raw != "" {
		var err error
		id, err = shared.ParsePositiveID(raw)
		if err != nil {
			shared.BadRequest(c, "记录 ID 无效")
			return
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var result interface{}
	var err error
	validation := true
	switch c.Param("kind") {
	case "tasks":
		var input model.ProductTask
		if c.ShouldBindJSON(&input) != nil {
			shared.BadRequest(c, "事项格式无效")
			return
		}
		input.ID, input.ProductID, input.CreatedAt, input.UpdatedAt = id, productID, time.Time{}, time.Time{}
		input.Title = strings.TrimSpace(input.Title)
		if !oneOf(input.Kind, "requirement", "bug", "action") || !oneOf(input.Status, "todo", "in_progress", "blocked", "done", "cancelled") || !oneOf(input.Priority, "high", "normal", "low") {
			shared.ValidationError(c, "事项类型、状态或优先级无效")
			return
		}
		if err = lifecycleText(input.Title, 512, true); err == nil {
			err = lifecycleText(input.Details, 8000, false)
		}
		if err == nil {
			err = lifecycleDate(input.DueDate)
		}
		if err == nil {
			validation = false
			err = h.repository.SaveProductTask(shared.UserID(c), &input)
		}
		result = &input
	case "versions":
		var input model.ProductVersion
		if c.ShouldBindJSON(&input) != nil {
			shared.BadRequest(c, "版本格式无效")
			return
		}
		input.ID, input.ProductID, input.CreatedAt, input.UpdatedAt = id, productID, time.Time{}, time.Time{}
		input.Name = strings.TrimSpace(input.Name)
		if !oneOf(input.Status, "planning", "developing", "testing", "releasing", "completed", "archived") || len(input.DeploymentIDs)+len(input.PlatformReleaseIDs) > 100 {
			shared.ValidationError(c, "版本状态无效或关联部署过多")
			return
		}
		err = lifecycleText(input.Name, 128, true)
		for _, field := range []struct {
			value string
			max   int
		}{{input.Goal, 1024}, {input.Acceptance, 8000}, {input.CommitSHA, 128}, {input.Notes, 8000}} {
			if err == nil {
				err = lifecycleText(field.value, field.max, false)
			}
		}
		if err == nil {
			err = lifecycleDate(input.TargetDate)
		}
		for _, url := range []string{input.RepositoryURL, input.BuildURL, input.ArtifactURL} {
			if err == nil {
				err = lifecycleText(url, 1024, false)
			}
			if err == nil {
				err = validateURL(url)
			}
		}
		if err == nil {
			validation = false
			err = h.repository.SaveProductVersion(shared.UserID(c), &input)
		}
		result = &input
	case "distributions":
		var input model.ProductDistribution
		if c.ShouldBindJSON(&input) != nil {
			shared.BadRequest(c, "发行记录格式无效")
			return
		}
		input.ID, input.ProductID, input.CreatedAt, input.UpdatedAt = id, productID, time.Time{}, time.Time{}
		input.PlatformVersion = strings.TrimSpace(input.PlatformVersion)
		if input.VersionID == 0 || input.ChannelID == 0 || !oneOf(input.Status, "draft", "preparing", "ready", "in_review", "approved", "published", "rejected", "blocked", "cancelled") {
			shared.ValidationError(c, "请选择产品版本、发行渠道和有效状态")
			return
		}
		if err = lifecycleText(input.PlatformVersion, 128, true); err == nil {
			err = lifecycleText(input.BuildIdentifier, 128, false)
		}
		if err == nil {
			err = lifecycleText(input.Notes, 8000, false)
		}
		if err == nil {
			validation = false
			err = h.repository.SaveProductDistribution(shared.UserID(c), &input)
		}
		result = &input
	default:
		shared.NotFound(c, "记录类型不存在")
		return
	}
	if err != nil {
		if validation {
			shared.ValidationError(c, err.Error())
		} else {
			respondError(c, err)
		}
		return
	}
	shared.Success(c, result)
}

func (h *Handler) DistributionEvents(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	recordID, err := shared.ParsePositiveID(c.Param("recordID"))
	if err != nil {
		shared.BadRequest(c, "发行记录 ID 无效")
		return
	}
	limit, offset := shared.LimitOffset(c, 10, 100)
	rows, count, err := h.repository.ListProductDistributionEvents(shared.UserID(c), id, recordID, model.ProductLifecycleFilter{Limit: limit, Offset: offset})
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, gin.H{"items": rows, "total": count, "limit": limit, "offset": offset})
}

func (h *Handler) ImportNextAction(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	task, err := h.repository.ImportProductNextAction(shared.UserID(c), id)
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, task)
}
