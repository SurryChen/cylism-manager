package cloud

import (
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	database "github.com/cylism/cylism-manager/internal/service/database"
	"github.com/gin-gonic/gin"
)

func (h *DatabaseHandler) MySQLClusterRoles(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if item.Status == "draft" || item.Status == "paused" {
		apiShared.Success(c, []database.MemberRole{})
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, "连接 Kubernetes 集群失败")
		return
	}
	roles, err := database.MemberRoles(c.Request.Context(), client, item)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, roles)
}
