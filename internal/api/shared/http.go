package shared

import (
	"net/http"
	"strconv"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

// ParseID parses a numeric route identifier. It deliberately keeps the
// historical behavior used by application and delivery handlers: zero is
// accepted here and endpoint-specific validation remains with the caller.
func ParseID(value string) (uint, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	return uint(id), err
}

// ParsePositiveID parses a route identifier and rejects zero, which is not a
// valid persisted resource ID for infrastructure endpoints.
func ParsePositiveID(value string) (uint, error) {
	id, err := ParseID(value)
	if err != nil || id == 0 {
		if err != nil {
			return 0, err
		}
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

// K8sUnavailable writes the stable API response used when the Kubernetes
// boundary has not been configured or is unavailable.
func K8sUnavailable(c *gin.Context) {
	model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
}
