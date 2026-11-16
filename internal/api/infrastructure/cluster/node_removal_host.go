package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// The host implementation only supports systemd workers using K3s's own
// containerd. It never runs the uninstall/killall scripts or removes data.
type sshNodeRemovalHost struct {
	key  []byte
	exec func(context.Context, time.Duration, *model.Server, []byte, string) ([]byte, error)
}

func removalHostCheckCommand(task *model.ClusterDeployment) string {
	command := `set -eu
test "$(cat /etc/machine-id)" = ` + shellEscape(task.NodeMachineID) + `
test "$(systemctl show k3s -p LoadState --value)" = not-found
test ! -d /var/lib/rancher/k3s/server/db
test "$(systemctl show k3s-agent -p LoadState --value)" = loaded
command -v k3s >/dev/null
# An active worker must expose K3s's private runtime; external CRI is unsupported.
state=$(systemctl is-active k3s-agent || true)
case "$state" in active) test -S /run/k3s/containerd/containerd.sock ;; inactive|failed) ;; *) exit 1 ;; esac
`
	if task.NodeStopped {
		command += `case "$state" in inactive|failed) ;; *) exit 1 ;; esac
enabled=$(systemctl is-enabled k3s-agent || true)
case "$enabled" in disabled|masked) ;; *) exit 1 ;; esac
`
	}
	return command
}

func (h *sshNodeRemovalHost) Check(ctx context.Context, server *model.Server, task *model.ClusterDeployment) error {
	if task.NodeMachineID == "" {
		return fmt.Errorf("缺少节点主机身份，不能停用服务")
	}
	if _, err := h.exec(ctx, 20*time.Second, server, h.key, privilegedCommand(server, removalHostCheckCommand(task))); err != nil {
		return fmt.Errorf("主机身份或服务检查失败；仅支持使用 systemd 的 K3s 工作节点，请核对 SSH、machine-id 和节点角色")
	}
	return nil
}

const removalHostStopCommand = `
systemctl disable --now k3s-agent
state=$(systemctl is-active k3s-agent || true)
case "$state" in inactive|failed) ;; *) exit 1 ;; esac
enabled=$(systemctl is-enabled k3s-agent || true)
case "$enabled" in disabled|masked) ;; *) exit 1 ;; esac
if [ -S /run/k3s/containerd/containerd.sock ]; then
  containers=$(k3s crictl --runtime-endpoint unix:///run/k3s/containerd/containerd.sock ps -q)
  for container in $containers; do
    k3s crictl --runtime-endpoint unix:///run/k3s/containerd/containerd.sock stop --timeout 30 "$container" >/dev/null
  done
  pods=$(k3s crictl --runtime-endpoint unix:///run/k3s/containerd/containerd.sock pods --state ready -q)
  for pod in $pods; do
    k3s crictl --runtime-endpoint unix:///run/k3s/containerd/containerd.sock stopp "$pod" >/dev/null
  done
  test -z "$(k3s crictl --runtime-endpoint unix:///run/k3s/containerd/containerd.sock ps -q)"
fi
# A missing/unavailable socket must not hide still-running K3s containers.
processes=$(ps -eo args=)
if printf '%s\n' "$processes" | awk '$1 ~ /(^|\/)containerd-shim/ && /\/run\/k3s\/containerd/ { found=1 } END { exit !found }'; then
  exit 1
fi
`

func (h *sshNodeRemovalHost) Stop(ctx context.Context, server *model.Server, task *model.ClusterDeployment) error {
	// Repeat the identity check in the same shell as the mutation.
	command := removalHostCheckCommand(task) + removalHostStopCommand
	if _, err := h.exec(ctx, 3*time.Minute, server, h.key, privilegedCommand(server, command)); err != nil {
		return fmt.Errorf("停用 k3s-agent 或清理残留容器失败；请检查 systemd 和 K3s 私有 containerd，平台关联已保留")
	}
	// Keep Tailscale online for server management, removing only this Node's
	// advertised Pod routes. Other subnet routes and exit-node settings survive.
	output, err := h.exec(ctx, 20*time.Second, server, h.key, privilegedCommand(server, "if command -v tailscale >/dev/null 2>&1; then tailscale debug prefs; else printf '%s' '{\"AdvertiseRoutes\":[]}'; fi"))
	if err != nil {
		return fmt.Errorf("读取 Tailscale 路由失败，尚未确认节点退出完成")
	}
	routes, changed, err := remainingRemovalRoutes(output, task.NodePodCIDRs)
	if err != nil {
		return err
	}
	if changed {
		// The CLI manages default routes through --advertise-exit-node, not
		// --advertise-routes. Leaving that option untouched preserves exit-node use.
		subnets := make([]string, 0, len(routes))
		for _, route := range routes {
			prefix, _ := netip.ParsePrefix(route)
			if prefix.Bits() != 0 {
				subnets = append(subnets, route)
			}
		}
		_, err = h.exec(ctx, 20*time.Second, server, h.key, privilegedCommand(server, "tailscale set --advertise-routes="+shellEscape(strings.Join(subnets, ","))))
		if err != nil {
			return fmt.Errorf("撤销节点 Pod 网段的 Tailscale 广告路由失败")
		}
	}
	return nil
}

func remainingRemovalRoutes(output []byte, podCIDRs string) ([]string, bool, error) {
	var prefs struct{ AdvertiseRoutes []string }
	if err := json.Unmarshal(output, &prefs); err != nil {
		return nil, false, fmt.Errorf("无法解析 Tailscale 路由状态")
	}
	var cidrs []string
	if err := json.Unmarshal([]byte(podCIDRs), &cidrs); err != nil {
		return nil, false, fmt.Errorf("任务缺少有效的 Pod 网段，请核对移出任务")
	}
	removed := make(map[netip.Prefix]bool, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, false, fmt.Errorf("节点 Pod 网段无效")
		}
		removed[prefix.Masked()] = true
	}
	remaining := make([]string, 0, len(prefs.AdvertiseRoutes))
	for _, route := range prefs.AdvertiseRoutes {
		prefix, err := netip.ParsePrefix(route)
		if err != nil {
			return nil, false, fmt.Errorf("Tailscale 广告路由无效")
		}
		if !removed[prefix.Masked()] {
			remaining = append(remaining, route)
		}
	}
	return remaining, len(remaining) != len(prefs.AdvertiseRoutes), nil
}

var _ NodeRemovalHost = (*sshNodeRemovalHost)(nil)
