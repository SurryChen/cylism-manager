package productapi

import (
	shared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type workspaceRepository interface {
	GetProjectWorkspace(uint, uint) (*model.Product, error)
	DeleteProjectWorkspace(uint, uint) error
}

func (h *Handler) Workspace(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	id, err := shared.ParsePositiveID(c.Param("projectID"))
	if err != nil {
		shared.BadRequest(c, "项目 ID 无效")
		return
	}
	repository, ok := h.repository.(workspaceRepository)
	if !ok {
		shared.DBError(c, "项目资料服务不可用")
		return
	}
	product, err := repository.GetProjectWorkspace(shared.UserID(c), id)
	if err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, product)
}

func (h *Handler) DeleteWorkspace(c *gin.Context) {
	if !authenticated(c) {
		return
	}
	id, err := shared.ParsePositiveID(c.Param("projectID"))
	if err != nil {
		shared.BadRequest(c, "项目 ID 无效")
		return
	}
	repository, ok := h.repository.(workspaceRepository)
	if !ok {
		shared.DBError(c, "项目资料服务不可用")
		return
	}
	if err := repository.DeleteProjectWorkspace(shared.UserID(c), id); err != nil {
		respondError(c, err)
		return
	}
	shared.Success(c, gin.H{"deleted": 1})
}
