package applicationapi

import (
	"context"
	"net"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

// IntegrationApplicationNetwork exposes deployment addressing without interpreting
// any application-specific protocol or configuration document.
func (h *ApplicationHandler) IntegrationApplicationNetwork(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	template, spec, err := h.integrationFileTemplate(app)
	if err != nil {
		integrationFileError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	apiShared.Success(c, gin.H{"host_network": spec.HostNetwork, "node_name": spec.NodeName, "public_host": h.applicationPublicHost(c.Request.Context(), app, spec), "template_revision": template.Revision})
}

// Host-network services may have no load-balancer address or Ingress. Only a
// public IP from the explicitly scheduled node is returned. Server inventory
// supplies the public host when Kubernetes only advertises an overlay address.
func (h *ApplicationHandler) applicationPublicHost(ctx context.Context, app *model.Application, spec applicationservice.ReleaseSpec) string {
	if !spec.HostNetwork || spec.NodeName == "" {
		return ""
	}
	var nodes []corev1.Node
	if h.clusterClient != nil {
		if client, err := h.clusterClient(ctx, app.Environment.ClusterID); err == nil && client != nil {
			nodes, _ = client.ListNodesContext(ctx)
		}
	}
	for _, node := range nodes {
		if node.Name != spec.NodeName {
			continue
		}
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeExternalIP && isPublicApplicationIP(address.Address) {
				return address.Address
			}
		}
	}
	if h.serverAddresses != nil {
		if servers, err := h.serverAddresses.ListServers(); err == nil {
			var host string
			for _, server := range servers {
				if server.ClusterID == nil || *server.ClusterID != app.Environment.ClusterID || server.K8sNodeName != spec.NodeName || !isPublicApplicationIP(server.Host) {
					continue
				}
				if host != "" && host != server.Host {
					return "" // Conflicting inventory must be resolved explicitly.
				}
				host = server.Host
			}
			if host != "" {
				return host
			}
		}
	}
	for _, node := range nodes {
		if node.Name == spec.NodeName {
			for _, address := range node.Status.Addresses {
				if address.Type == corev1.NodeInternalIP && isPublicApplicationIP(address.Address) {
					return address.Address
				}
			}
		}
	}
	return ""
}

func isPublicApplicationIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	// IsPrivate does not cover shared address space, including overlay networks.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}
