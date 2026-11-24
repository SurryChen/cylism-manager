package applicationapi

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// workspaceScope resolves the real runtime environment and filters applications
// by their management project. Physical environment queries remain independent.
func (h *ApplicationHandler) workspaceScope(c *gin.Context, projectID, environmentID uint) (*model.Environment, []model.Application, []string, bool) {
	environment, localErr := h.queries.ResolveEnvironment(projectID, environmentID)
	physical, err := h.queries.ListApplications(0, environmentID)
	if err != nil {
		apiShared.DBError(c, "读取应用失败")
		return nil, nil, nil, false
	}
	ownerID := apiShared.UserID(c)
	var names []string
	apps := make([]model.Application, 0, len(physical))
	for _, app := range physical {
		if localErr == nil && app.ProjectID == projectID {
			names = append(names, app.Name)
		}
		if app.OwningProjectID() != projectID {
			continue
		}
		if localErr != nil {
			if ownerID == 0 || app.Project.OwnerID != ownerID || app.Environment.ID != environmentID || app.Environment.ProjectID != app.ProjectID {
				continue
			}
			environment = &app.Environment
		}
		apps = append(apps, app)
	}
	if localErr != nil {
		project, err := h.queries.GetProject(projectID)
		if environment == nil || err != nil || project.OwnerID != ownerID {
			apiShared.ValidationError(c, "环境不属于所选项目")
			return nil, nil, nil, false
		}
	}
	return environment, apps, names, true
}
