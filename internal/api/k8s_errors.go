package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// k8sUnavailable is shared by legacy HTTP adapters that still expose
// non-infrastructure endpoints. The infrastructure K8s handler owns its own
// copy so the packages remain acyclic.
func k8sUnavailable(c *gin.Context) {
	model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
}
