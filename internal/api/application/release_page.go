package applicationapi

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

// ListReleasePage shares the application's existing authorization boundary.
func (h *ApplicationHandler) ListReleasePage(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	if _, err := h.queries.GetApplication(id); err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	h.releasePage(c, []uint{id}, nil)
}

func (h *ApplicationHandler) WorkspaceReleasePage(c *gin.Context) {
	projectID, err := apiShared.ParsePositiveID(c.Query("project_id"))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	environmentID, err := apiShared.ParsePositiveID(c.Query("environment_id"))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	if _, err := h.queries.GetProject(projectID); err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	if _, err := h.queries.ResolveEnvironment(projectID, environmentID); err != nil {
		apiShared.BadRequest(c, "环境不属于所选项目")
		return
	}
	apps, err := h.queries.ListApplications(projectID, environmentID)
	if err != nil {
		apiShared.DBError(c, "读取应用失败")
		return
	}
	ids := make([]uint, 0, len(apps))
	names := map[uint]string{}
	for _, app := range apps {
		ids = append(ids, app.ID)
		names[app.ID] = app.Name
	}
	h.releasePage(c, ids, names)
}

func (h *ApplicationHandler) releasePage(c *gin.Context, ids []uint, names map[uint]string) {
	limit, offset := apiShared.LimitOffset(c, 10, 100)
	releases, total, err := h.queries.ListReleasesPage(ids, limit, offset)
	if err != nil {
		apiShared.DBError(c, "读取发布记录失败")
		return
	}
	if names == nil {
		apiShared.Success(c, gin.H{"items": apiShared.ReleasesDTO(releases), "total": total, "limit": limit, "offset": offset})
		return
	}
	items := make([]workspaceReleaseSummary, 0, len(releases))
	for _, release := range releases {
		items = append(items, workspaceReleaseSummary{ID: release.ID, ApplicationID: release.ApplicationID, ApplicationName: names[release.ApplicationID], Sequence: release.Sequence, Version: release.Version, Image: release.Image, Status: release.Status, CreatedAt: release.CreatedAt})
	}
	apiShared.Success(c, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}
