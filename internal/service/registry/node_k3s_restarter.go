package registry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/repository"
)

const (
	NodeK3sRestartStatusSucceeded   = "succeeded"
	NodeK3sRestartStatusFailed      = "failed"
	NodeK3sRestartStatusUnsupported = "unsupported"
)

// NodeK3sServiceRestarter is constrained to a managed node ID and to the K3s
// service units. It deliberately cannot reboot hosts or execute caller input.
type NodeK3sServiceRestarter interface {
	Restart(context.Context, uint) (NodeK3sRestartResult, error)
}

type NodeK3sRestartResult struct {
	ServerID uint   `json:"server_id"`
	Service  string `json:"service,omitempty"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

type NodeK3sRestarter struct {
	repository repository.NodeRegistryMirrorRepository
	ssh        SSHExecutor
}

func NewNodeK3sServiceRestarter(repository repository.NodeRegistryMirrorRepository, ssh SSHExecutor) *NodeK3sRestarter {
	return &NodeK3sRestarter{repository: repository, ssh: ssh}
}

func (s *NodeK3sRestarter) Restart(ctx context.Context, serverID uint) (NodeK3sRestartResult, error) {
	if s == nil || s.repository == nil {
		return NodeK3sRestartResult{}, fmt.Errorf("节点 K3s 重启服务不可用")
	}
	servers, err := s.repository.ListServers()
	if err != nil {
		return NodeK3sRestartResult{}, err
	}
	for _, server := range servers {
		if server.ID != serverID {
			continue
		}
		if strings.TrimSpace(server.ClusterRole) == "" {
			return NodeK3sRestartResult{}, fmt.Errorf("节点不是可重启的集群节点")
		}
		if server.SSHAuthType != "key" || strings.TrimSpace(server.SSHKey) == "" {
			return NodeK3sRestartResult{ServerID: serverID, Status: NodeK3sRestartStatusUnsupported, Detail: "需要已配置的 SSH 密钥认证"}, nil
		}
		if s.ssh == nil {
			return NodeK3sRestartResult{ServerID: serverID, Status: NodeK3sRestartStatusFailed, Detail: "节点 SSH 重启通道不可用"}, nil
		}
		out, executeErr := s.ssh.Execute(ctx, 90*time.Second, &server, nodeK3sRestartCommand())
		if executeErr != nil {
			return NodeK3sRestartResult{ServerID: serverID, Status: NodeK3sRestartStatusFailed, Detail: "K3s 服务重启失败"}, nil
		}
		service := strings.TrimSpace(string(out))
		if service != "k3s.service" && service != "k3s-agent.service" {
			service = ""
		}
		return NodeK3sRestartResult{ServerID: serverID, Service: service, Status: NodeK3sRestartStatusSucceeded, Detail: "K3s 服务已重启并恢复运行"}, nil
	}
	return NodeK3sRestartResult{}, fmt.Errorf("节点不是可重启的集群节点")
}

func nodeK3sRestartCommand() string {
	return `set -e
unit=""
for candidate in k3s.service k3s-agent.service; do
  if [ "$(sudo -n systemctl show "$candidate" -p LoadState --value 2>/dev/null)" = "loaded" ]; then
    unit="$candidate"
    break
  fi
done
if [ -z "$unit" ]; then
  exit 1
fi
sudo -n systemctl restart "$unit"
sudo -n systemctl is-active --quiet "$unit"
printf '%s\n' "$unit"`
}
