package applicationapi

import (
	"strconv"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

func (h *ApplicationHandler) IntegrationListApplicationPods(c *gin.Context) {
	if _, ok := h.integrationApplication(c, "application:read"); !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	h.ListApplicationPods(c)
}

func (h *ApplicationHandler) IntegrationGetApplicationPodLogs(c *gin.Context) {
	if _, ok := h.integrationApplication(c, "application:logs"); !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	// The shared reader additionally verifies Pod ownership and container names,
	// and limits the request to at most 500 lines without streaming.
	h.GetApplicationPodLogs(c)
}

func (h *ApplicationHandler) IntegrationListApplicationReleases(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	limit := 20
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 50 {
			apiShared.BadRequest(c, "发布记录数量必须在 1 到 50 之间")
			return
		}
		limit = parsed
	}
	releases, err := h.queries.ListReleases(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	type summary struct {
		applicationReleaseSummary
		CreatedAt time.Time `json:"created_at"`
	}
	items := make([]summary, 0, min(limit, len(releases)))
	for _, release := range releases {
		if len(items) == limit {
			break
		}
		items = append(items, summary{applicationReleaseSummary: applicationReleaseSummary{ID: release.ID, Sequence: release.Sequence, Version: release.Version, Status: release.Status}, CreatedAt: release.CreatedAt})
	}
	c.Header("Cache-Control", "no-store")
	apiShared.Success(c, items)
}
