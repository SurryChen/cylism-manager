package kubernetes

import (
	"context"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/gin-gonic/gin"
)

const platformIdentityTimeout = 3 * time.Second

type PlatformIdentityReader interface {
	ServerVersionContext(context.Context) (string, error)
}

// ClusterPlatformHandler exposes safe connected-cluster identity metadata.
type ClusterPlatformHandler struct {
	reader PlatformIdentityReader
}

func NewClusterPlatformHandler(reader PlatformIdentityReader) *ClusterPlatformHandler {
	return &ClusterPlatformHandler{reader: reader}
}

func (h *ClusterPlatformHandler) Get(c *gin.Context) {
	if h == nil || h.reader == nil {
		apiShared.Success(c, platformIdentityResponse("unknown", "", "unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), platformIdentityTimeout)
	defer cancel()
	version, err := h.reader.ServerVersionContext(ctx)
	if err != nil {
		apiShared.Success(c, platformIdentityResponse("unknown", "", "unavailable"))
		return
	}
	distribution := classifyPlatform(version)
	apiShared.Success(c, platformIdentityResponse(distribution, strings.TrimSpace(version), ""))
}

func platformIdentityResponse(distribution, version, reason string) gin.H {
	isK3s := distribution == "k3s"
	return gin.H{
		"distribution": distribution,
		"version":      version,
		"reason":       reason,
		"capabilities": gin.H{
			"k3s_node_join": isK3s,
		},
	}
}

func classifyPlatform(version string) string {
	if strings.TrimSpace(version) == "" {
		return "unknown"
	}
	if strings.Contains(strings.ToLower(version), "k3s") {
		return "k3s"
	}
	return "kubernetes"
}
