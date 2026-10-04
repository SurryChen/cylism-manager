package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/cylism/cylism-manager/internal/clusteragent"
	"github.com/cylism/cylism-manager/internal/k8s"
	accesscert "github.com/cylism/cylism-manager/internal/service/accesscert"
	"github.com/cylism/cylism-manager/internal/store"
)

// clusterAgentClientFactory resolves a cluster's control-plane server and
// creates a short-lived Kubernetes client over the Agent mTLS endpoint.
// Client handles are cheap and share the underlying HTTP transport for the
// duration of the request; certificates remain encrypted at rest in Store.
type clusterAgentClientFactory struct {
	store *store.Store
	certs *accesscert.Service
}

func newClusterAgentClientFactory(st *store.Store, certs *accesscert.Service) *clusterAgentClientFactory {
	return &clusterAgentClientFactory{store: st, certs: certs}
}

func (f *clusterAgentClientFactory) Client(ctx context.Context, clusterID uint) (*k8s.Client, error) {
	_ = ctx
	if f == nil || f.store == nil || f.certs == nil {
		return nil, fmt.Errorf("集群 Agent 访问未配置")
	}
	cluster, err := f.store.GetCluster(clusterID)
	if err != nil {
		return nil, fmt.Errorf("读取集群失败: %w", err)
	}
	if cluster.ControlServerID == nil {
		return nil, fmt.Errorf("集群尚未配置控制平面服务器")
	}
	server, err := f.store.GetServer(*cluster.ControlServerID)
	if err != nil {
		return nil, fmt.Errorf("读取控制平面服务器失败: %w", err)
	}
	// Agent certificates are issued with the control address when one is
	// configured, which is also the address used by the K3s join workflow.
	// Older/adopted clusters may lack it, so fall back to the server host.
	endpoint := agentEndpoint(cluster.ControlAddress)
	if endpoint == "" {
		endpoint = agentEndpoint(server.Host)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("未配置控制平面地址")
	}
	material, err := f.certs.PlatformClientTLS(clusterID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return nil, fmt.Errorf("请先在访问证书页面创建访问 CA")
		}
		return nil, fmt.Errorf("准备平台客户端证书失败: %w", err)
	}
	return clusteragent.NewKubernetesClient(endpoint, material)
}

func agentEndpoint(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return "https://" + net.JoinHostPort(parsed.Hostname(), "9443")
}
