package productapi

import (
	"fmt"
	"net/http"
	"strconv"
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
	DeleteProductTask(uint, uint, uint) error
	ProductTaskGroups(uint, uint) ([]model.ProductTaskGroup, error)
	SaveProductTaskGroup(uint, *model.ProductTaskGroup) error
	DeleteProductTaskGroup(uint, uint, uint) error
	SaveProductVersion(uint, *model.ProductVersion) error
	SaveProductDistribution(uint, *model.ProductDistribution) error
	SaveProductReleaseUnit(uint, *model.ProductReleaseUnit) error
	DeleteProductReleaseUnit(uint, uint, uint) error
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
		ID            uint   `json:"id"`
		Name          string `json:"name"`
		Status        string `json:"status"`
		ReleaseUnitID uint   `json:"release_unit_id"`
		ReleasedOn    string `json:"released_on"`
	}
	options := []option{}
	for _, row := range rows {
		options = append(options, option{row.ID, row.Name, row.Status, row.ReleaseUnitID, row.ReleasedOn})
	}
	groups, err := h.repository.ProductTaskGroups(shared.UserID(c), id)
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, gin.H{"versions": options, "task_groups": groups})
}

func (h *Handler) ListLifecycle(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	limit, offset := shared.LimitOffset(c, 10, 100)
	f := model.ProductLifecycleFilter{Audience: c.Query("audience"), Status: c.Query("status"), Keyword: strings.TrimSpace(c.Query("keyword")), Limit: limit, Offset: offset}
	if raw := c.Query("version_id"); raw != "" {
		var err error
		f.VersionID, err = shared.ParsePositiveID(raw)
		if err != nil {
			shared.BadRequest(c, "版本 ID 无效")
			return
		}
	}
	kind := c.Param("kind")
	if raw := c.Query("release_unit_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, strconv.IntSize)
		if err != nil {
			shared.BadRequest(c, "发布端 ID 无效")
			return
		}
		unitID := uint(value)
		f.ReleaseUnitID = &unitID
	}
	if raw := c.Query("environment_id"); raw != "" {
		value, err := shared.ParsePositiveID(raw)
		if err != nil {
			shared.BadRequest(c, "环境 ID 无效")
			return
		}
		f.EnvironmentID = &value
	}
	if kind == "versions" {
		f.VersionID = 0
	}
	var rows interface{}
	var count int64
	var err error
	switch kind {
	case "task-groups":
		var groups []model.ProductTaskGroup
		groups, err = h.repository.ProductTaskGroups(shared.UserID(c), id)
		count = int64(len(groups))
		start := min(offset, len(groups))
		rows = groups[start:min(start+limit, len(groups))]
	case "tasks":
		if raw := c.Query("group_id"); raw != "" {
			value, parseErr := strconv.ParseUint(raw, 10, strconv.IntSize)
			if parseErr != nil {
				shared.BadRequest(c, "待办分组 ID 无效")
				return
			}
			groupID := uint(value)
			f.TaskGroupID = &groupID
		}
		f.OpenTasksOnly = c.Query("open") == "true"
		f.AssignableTasks = c.Query("assignable") == "true"
		rows, count, err = h.repository.ListProductTasks(shared.UserID(c), id, f)
	case "versions":
		rows, count, err = h.repository.ListProductVersions(shared.UserID(c), id, f)
	case "distributions":
		rows, count, err = h.repository.ListProductDistributions(shared.UserID(c), id, f)
	case "deployments":
		f.IncludeVersionDeployments = c.Query("include_available") == "true"
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
	case "release-units":
		var input model.ProductReleaseUnit
		if c.ShouldBindJSON(&input) != nil {
			shared.BadRequest(c, "发布端格式无效")
			return
		}
		input.ID, input.ProductID = id, productID
		input.Name = strings.TrimSpace(input.Name)
		err = lifecycleText(input.Name, 128, true)
		if err == nil {
			validation = false
			err = h.repository.SaveProductReleaseUnit(shared.UserID(c), &input)
		}
		result = &input
	case "task-groups":
		var input struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&input) != nil {
			shared.BadRequest(c, "待办分组格式无效")
			return
		}
		group := model.ProductTaskGroup{ID: id, ProductID: productID, Name: strings.TrimSpace(input.Name)}
		if err = lifecycleText(group.Name, 64, true); err == nil {
			validation = false
			err = h.repository.SaveProductTaskGroup(shared.UserID(c), &group)
		}
		result = &group
	case "tasks":
		var payload struct {
			model.ProductTask
			GroupID *uint `json:"group_id"`
		}
		if c.ShouldBindJSON(&payload) != nil {
			shared.BadRequest(c, "事项格式无效")
			return
		}
		input := payload.ProductTask
		input.KeepGroup = payload.GroupID == nil
		if payload.GroupID != nil {
			input.GroupID = *payload.GroupID
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
		var payload struct {
			model.ProductVersion
			ReleaseUnitID *uint               `json:"release_unit_id"`
			GitHubSources []githubSourceInput `json:"github_sources"`
		}
		if c.ShouldBindJSON(&payload) != nil {
			shared.BadRequest(c, "版本格式无效")
			return
		}
		input := payload.ProductVersion
		input.KeepReleaseUnit = payload.ReleaseUnitID == nil
		if payload.ReleaseUnitID != nil {
			input.ReleaseUnitID = *payload.ReleaseUnitID
		}
		input.ID, input.ProductID, input.CreatedAt, input.UpdatedAt = id, productID, time.Time{}, time.Time{}
		input.Name = strings.TrimSpace(input.Name)
		if input.ReleasedOn != "" {
			input.Status = "completed"
		}
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
		if err == nil {
			err = lifecycleDate(input.ReleasedOn)
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
			if sourceErr := resolveGitHubSources(c.Request.Context(), h.github, shared.UserID(c), &input, payload.GitHubSources); sourceErr != nil {
				respondGitHubSourceError(c, sourceErr)
				return
			}
		}
		if err == nil {
			validation = false
			err = h.repository.SaveProductVersion(shared.UserID(c), &input)
		}
		result = &input
	case "distributions":
		var payload struct {
			model.ProductDistribution
			EnvironmentID *uint   `json:"environment_id"`
			DeploymentIDs *[]uint `json:"deployment_ids"`
		}
		if c.ShouldBindJSON(&payload) != nil {
			shared.BadRequest(c, "发行记录格式无效")
			return
		}
		input := payload.ProductDistribution
		input.WeChatPublishedAt = nil
		input.KeepEnvironment = payload.EnvironmentID == nil
		if payload.EnvironmentID != nil {
			input.EnvironmentID = *payload.EnvironmentID
		}
		input.KeepDeployments = payload.DeploymentIDs == nil
		if payload.DeploymentIDs != nil {
			input.DeploymentIDs = *payload.DeploymentIDs
		}
		input.ID, input.ProductID, input.CreatedAt, input.UpdatedAt = id, productID, time.Time{}, time.Time{}
		input.PlatformVersion = strings.TrimSpace(input.PlatformVersion)
		if input.Audience == "" {
			input.Audience = "unclassified"
		}
		if !oneOf(input.Audience, "unclassified", "testing", "production") {
			shared.ValidationError(c, "发行分类无效")
			return
		}
		if input.ChannelID == 0 || !oneOf(input.Status, "draft", "preparing", "ready", "in_review", "approved", "published", "rejected", "blocked", "cancelled") || len(input.DeploymentIDs) > 100 {
			shared.ValidationError(c, "请选择发行渠道和有效状态")
			return
		}
		if input.VersionID == 0 && input.PlatformVersion == "" {
			shared.ValidationError(c, "请选择正式版本，或填写渠道版本号")
			return
		}
		if err = lifecycleText(input.PlatformVersion, 128, false); err == nil {
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

func (h *Handler) DeleteTaskGroup(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	groupID, err := shared.ParsePositiveID(c.Param("recordID"))
	if err != nil {
		shared.BadRequest(c, "待办分组 ID 无效")
		return
	}
	if err := h.repository.DeleteProductTaskGroup(shared.UserID(c), id, groupID); err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, gin.H{"deleted": 1})
}

func (h *Handler) DeleteTask(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	taskID, err := shared.ParsePositiveID(c.Param("recordID"))
	if err != nil {
		shared.BadRequest(c, "事项 ID 无效")
		return
	}
	if err := h.repository.DeleteProductTask(shared.UserID(c), id, taskID); err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, nil)
}

func (h *Handler) DeleteReleaseUnit(c *gin.Context) {
	id, ok := lifecycleID(c)
	if !ok {
		return
	}
	unitID, err := shared.ParsePositiveID(c.Param("recordID"))
	if err != nil {
		shared.BadRequest(c, "发布端 ID 无效")
		return
	}
	if err := h.repository.DeleteProductReleaseUnit(shared.UserID(c), id, unitID); err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, nil)
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
