package system

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// k8sClient is the system API's Kubernetes boundary. It is assigned once during
// route registration and kept here so system handlers do not depend on the
// root api package.
var k8sClient *k8s.Client

// SetKubernetesClient injects the process Kubernetes boundary used by system
// handlers and the background system-component reconciler.
func SetKubernetesClient(client *k8s.Client) {
	k8sClient = client
}

func k8sUnavailable(c *gin.Context) {
	model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "k8sClient 集群未连接")
}
