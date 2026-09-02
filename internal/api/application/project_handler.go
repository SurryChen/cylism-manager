package applicationapi

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type projectRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	DefaultImageRegistryID *uint  `json:"default_image_registry_id"`
}

func (h *ApplicationHandler) ListProjects(c *gin.Context) {
	projects, err := h.applications.ListProjects()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, projects)
}

func (h *ApplicationHandler) CreateProject(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		apiShared.BadRequest(c, "项目名称必填")
		return
	}
	if req.DefaultImageRegistryID != nil && *req.DefaultImageRegistryID != 0 {
		apiShared.BadRequest(c, "请在项目创建后设置默认镜像仓库")
		return
	}
	project := &model.Project{Name: req.Name, Description: req.Description, OwnerID: apiShared.UserID(c)}
	if err := h.applications.CreateProject(project); err != nil {
		apiShared.Conflict(c, "项目名称已存在")
		return
	}
	model.Success(c, project)
}

func (h *ApplicationHandler) UpdateProject(c *gin.Context) {
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		apiShared.BadRequest(c, "项目名称必填")
		return
	}
	project, err := h.queries.GetProject(projectID)
	if err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	if project.Name != req.Name {
		applicationCount, err := h.applications.CountProjectApplications(projectID)
		if err != nil {
			apiShared.DBError(c, err.Error())
			return
		}
		if applicationCount > 0 {
			apiShared.Conflict(c, "项目已有应用，不能修改项目名称")
			return
		}
	}
	project.Name = req.Name
	project.Description = req.Description
	if req.DefaultImageRegistryID != nil {
		if *req.DefaultImageRegistryID == 0 {
			project.DefaultImageRegistryID = nil
		} else {
			registry, err := h.resources.GetImageRegistryForProject(*req.DefaultImageRegistryID, projectID)
			if err != nil || !registry.Enabled {
				apiShared.BadRequest(c, "默认镜像仓库未授权当前项目或已停用")
				return
			}
			project.DefaultImageRegistryID = &registry.ID
		}
	}
	if err := h.applications.UpdateProject(project); err != nil {
		apiShared.Conflict(c, "项目名称已存在")
		return
	}
	model.Success(c, project)
}

func (h *ApplicationHandler) DeleteProject(c *gin.Context) {
	projectID, err := apiShared.ParseID(c.Param("projectID"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	if _, err := h.queries.GetProject(projectID); err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	environmentCount, err := h.applications.CountProjectEnvironments(projectID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	applicationCount, err := h.applications.CountProjectApplications(projectID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if environmentCount > 0 || applicationCount > 0 {
		apiShared.Conflict(c, "项目仍关联环境或应用，无法删除")
		return
	}
	if err := h.applications.DeleteProject(projectID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"id": projectID})
}
