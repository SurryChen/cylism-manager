package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// NodeMirrorApplierService applies rendered K3s registry configuration to a
// selected node. SSH transport is injected so this service remains independent
// of the concrete SSH implementation and of HTTP packages.
type NodeMirrorApplierService struct {
	ssh SSHExecutor
}

func NewNodeMirrorApplier(ssh SSHExecutor) *NodeMirrorApplierService {
	return &NodeMirrorApplierService{ssh: ssh}
}

// Apply writes registries.yaml on the node and restarts the appropriate K3s
// service. The payload is encoded before being embedded in the shell command.
func (s *NodeMirrorApplierService) Apply(ctx context.Context, server *model.Server, content []byte) (string, string) {
	if s == nil || s.ssh == nil {
		return "failed", "节点镜像源应用能力不可用"
	}
	if server == nil {
		return "failed", "节点不可用"
	}
	if server.SSHAuthType != "key" || strings.TrimSpace(server.SSHKey) == "" {
		return "skipped", "需要已配置的 SSH 密钥认证"
	}
	payload := base64.StdEncoding.EncodeToString(content)
	out, err := s.ssh.Execute(ctx, 90*time.Second, server, nodeRegistryMirrorApplyCommand(payload))
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return "failed", fmt.Sprintf("节点镜像源应用失败: %v", err)
		}
		return "failed", fmt.Sprintf("节点镜像源应用失败: %v: %s", err, detail)
	}
	return "success", strings.TrimSpace(string(out))
}

func nodeRegistryMirrorApplyCommand(payload string) string {
	return fmt.Sprintf(`set -e
sudo -n mkdir -p /etc/rancher/k3s
target="/etc/rancher/k3s/registries.yaml"
backup="${target}.cylism-backup"
tmp="${target}.tmp"
if sudo -n test -f "$target"; then
  sudo -n cp "$target" "$backup"
fi
echo %q | base64 -d | sudo -n tee "$tmp" >/dev/null
sudo -n chmod 600 "$tmp"
sudo -n mv "$tmp" "$target"
unit=""
for candidate in k3s k3s-agent; do
  if [ "$(sudo -n systemctl show "${candidate}.service" -p LoadState --value 2>/dev/null)" = "loaded" ]; then
    unit="${candidate}.service"
    break
  fi
done
if [ -z "$unit" ]; then
  echo "未找到 k3s.service 或 k3s-agent.service" >&2
  exit 1
fi
echo "检测到服务: $unit"
restart_failed=0
if ! sudo -n systemctl restart "$unit"; then
  restart_failed=1
fi
if ! sudo -n systemctl is-active --quiet "$unit"; then
  restart_failed=1
fi
if [ "$restart_failed" -ne 0 ]; then
  echo "服务重启命令失败: $unit" >&2
  sudo -n systemctl status "$unit" --no-pager -l >&2 || true
  if sudo -n test -f "$backup"; then
    echo "正在恢复旧配置: $target" >&2
    sudo -n cp "$backup" "$target"
    sudo -n systemctl restart "$unit" || true
  fi
  exit 1
fi
echo "服务已重启: $unit"`, payload)
}
