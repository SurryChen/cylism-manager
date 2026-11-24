package applicationapi

import (
	"errors"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *ApplicationHandler) ApplicationProjectTransferOptions(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	ownerID := apiShared.UserID(c)
	app, err := h.queries.GetApplication(id)
	if err != nil || ownerID == 0 || app.Project.OwnerID != ownerID {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	current, err := h.queries.GetProject(app.OwningProjectID())
	if err != nil || current.OwnerID != ownerID {
		apiShared.NotFound(c, "所属项目不存在")
		return
	}
	projects, err := h.applications.ListApplicationTransferProjects(ownerID)
	if err != nil {
		apiShared.DBError(c, "读取可迁移项目失败")
		return
	}
	type projectOption struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	options := make([]projectOption, 0, len(projects))
	for _, project := range projects {
		if project.ID != current.ID {
			options = append(options, projectOption{project.ID, project.Name})
		}
	}
	apiShared.Success(c, gin.H{"current_project": projectOption{current.ID, current.Name}, "targets": options})
}

func (h *ApplicationHandler) TransferApplicationProject(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	var request struct {
		FromProjectID uint `json:"from_project_id" binding:"required"`
		ToProjectID   uint `json:"to_project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.FromProjectID == request.ToProjectID {
		apiShared.BadRequest(c, "请选择不同的目标项目")
		return
	}
	err = h.applications.TransferApplicationProject(model.ApplicationProjectTransfer{
		ApplicationID: id, OwnerID: apiShared.UserID(c), FromProjectID: request.FromProjectID, ToProjectID: request.ToProjectID,
	})
	switch {
	case err == nil:
		apiShared.Success(c, gin.H{"project_id": request.ToProjectID})
	case errors.Is(err, gorm.ErrRecordNotFound):
		apiShared.NotFound(c, "应用或所属项目不存在")
	case errors.Is(err, model.ErrApplicationTransferTarget):
		apiShared.ValidationError(c, err.Error())
	case errors.Is(err, model.ErrApplicationProjectChanged), errors.Is(err, model.ErrApplicationTransferBusy):
		apiShared.Conflict(c, err.Error())
	default:
		apiShared.DBError(c, "迁移应用失败")
	}
}
