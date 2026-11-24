package applicationapi

import (
	"fmt"
	"sort"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type workspaceEndpointRow struct {
	Key         string                             `json:"key"`
	Application workspaceApplicationInfo           `json:"app"`
	Endpoint    *apiShared.ApplicationEndpointView `json:"endpoint"`
}

// Page after computing the environment-wide runtime summary. This preserves
// live-pod release selection and keeps readiness independent of the current page.
func (h *ApplicationHandler) workspaceApplicationPage(c *gin.Context, namespace string, applications []model.Application, names []string) {
	view := c.DefaultQuery("view", "runtime")
	if view != "runtime" && view != "config" && view != "access" {
		apiShared.BadRequest(c, "应用部署视图无效")
		return
	}
	sort.SliceStable(applications, func(i, j int) bool {
		if applications[i].CreatedAt.Equal(applications[j].CreatedAt) {
			return applications[i].ID > applications[j].ID
		}
		return applications[i].CreatedAt.After(applications[j].CreatedAt)
	})
	ids := make([]uint, 0, len(applications))
	for _, app := range applications {
		ids = append(ids, app.ID)
	}
	releases, err := h.queries.ListReleasesByApplications(ids)
	if err != nil {
		apiShared.DBError(c, "读取发布记录失败")
		return
	}
	for i := range applications {
		for j := range applications[i].Endpoints {
			applications[i].Endpoints[j].IngressEnabled = endpointUsesIngress(applications[i].Endpoints[j])
		}
	}
	infos := h.workspaceApplicationInfos(c.Request.Context(), namespace, applications, releases)
	ready := 0
	for _, info := range infos {
		if info.Runtime.Status == "running" {
			ready++
		}
	}
	limit, offset := apiShared.LimitOffset(c, 10, 100)
	response := gin.H{"applications_total": len(infos), "ready_applications": ready, "application_names": append([]string{}, names...), "limit": limit, "offset": offset}
	if view == "access" {
		rows := make([]workspaceEndpointRow, 0)
		for i, app := range applications {
			if len(app.Endpoints) == 0 {
				rows = append(rows, workspaceEndpointRow{Key: fmt.Sprintf("%d-none", app.ID), Application: infos[i]})
				continue
			}
			for _, endpoint := range infos[i].Endpoints {
				// Reuse the sanitized API endpoint view rather than exposing persistence fields.
				rows = append(rows, workspaceEndpointRow{Key: fmt.Sprintf("%d-%d", app.ID, endpoint.ID), Application: infos[i], Endpoint: &endpoint})
			}
		}
		start := min(offset, len(rows))
		response["items"], response["total"] = rows[start:min(start+limit, len(rows))], len(rows)
	} else {
		start := min(offset, len(infos))
		response["items"], response["total"] = infos[start:min(start+limit, len(infos))], len(infos)
	}
	apiShared.Success(c, response)
}
