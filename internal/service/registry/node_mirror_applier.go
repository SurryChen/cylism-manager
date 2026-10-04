package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"sigs.k8s.io/yaml"
)

// NodeMirrorApplierService applies rendered K3s registry configuration to a
// selected node. SSH transport is injected so this service remains independent
// of the concrete SSH implementation and of HTTP packages.
type NodeMirrorApplierService struct {
	ssh        SSHExecutor
	repository repository.NodeRegistryMirrorRepository
}

func NewNodeMirrorApplier(ssh SSHExecutor) *NodeMirrorApplierService {
	return &NodeMirrorApplierService{ssh: ssh}
}

func NewNodeMirrorApplierWithRepository(ssh SSHExecutor, repo repository.NodeRegistryMirrorRepository) *NodeMirrorApplierService {
	return &NodeMirrorApplierService{ssh: ssh, repository: repo}
}

type NodeRegistryConfigSyncer interface {
	Sync(context.Context, *model.Server, []*model.Server) ([]NodeRegistryConfigSyncResult, error)
	Update(context.Context, []*model.Server, []SafeRegistryConfig) ([]NodeRegistryConfigSyncResult, error)
}

type NodeRegistryConfigSyncResult struct {
	ServerID uint   `json:"server_id"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

func (s *NodeMirrorApplierService) Sync(ctx context.Context, source *model.Server, targets []*model.Server) ([]NodeRegistryConfigSyncResult, error) {
	if s == nil || s.ssh == nil || source == nil || len(targets) == 0 {
		return nil, fmt.Errorf("节点配置同步参数无效")
	}
	raw, err := s.ssh.Execute(ctx, 30*time.Second, source, nodeRegistryConfigReadCommand())
	if err != nil {
		return nil, fmt.Errorf("读取源节点镜像源配置失败: %w", err)
	}
	return s.writeToTargets(ctx, targets, raw), nil
}

func (s *NodeMirrorApplierService) Update(ctx context.Context, targets []*model.Server, configs []SafeRegistryConfig) ([]NodeRegistryConfigSyncResult, error) {
	if s == nil || s.ssh == nil || len(targets) == 0 {
		return nil, fmt.Errorf("节点配置编辑参数无效")
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("至少配置一个镜像源")
	}
	results := make([]NodeRegistryConfigSyncResult, 0, len(targets))
	for _, target := range targets {
		raw, err := s.ssh.Execute(ctx, 30*time.Second, target, nodeRegistryConfigReadCommand())
		if err != nil {
			results = append(results, NodeRegistryConfigSyncResult{ServerID: target.ID, Status: "failed", Detail: "读取节点原有配置失败"})
			continue
		}
		updated, err := mergeSafeRegistryConfigs(raw, configs)
		if err != nil {
			results = append(results, NodeRegistryConfigSyncResult{ServerID: target.ID, Status: "failed", Detail: err.Error()})
			continue
		}
		result, writeErr := s.writeToTarget(ctx, target, updated)
		if writeErr != nil {
			result.Detail = writeErr.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *NodeMirrorApplierService) writeToTargets(ctx context.Context, targets []*model.Server, content []byte) []NodeRegistryConfigSyncResult {
	results := make([]NodeRegistryConfigSyncResult, 0, len(targets))
	for _, target := range targets {
		result, err := s.writeToTarget(ctx, target, content)
		if err != nil {
			result.Detail = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (s *NodeMirrorApplierService) writeToTarget(ctx context.Context, target *model.Server, content []byte) (NodeRegistryConfigSyncResult, error) {
	result := NodeRegistryConfigSyncResult{ServerID: target.ID, Status: "failed"}
	payload := base64.StdEncoding.EncodeToString(content)
	out, err := s.ssh.Execute(ctx, 90*time.Second, target, nodeRegistryConfigWriteCommand(payload))
	if err != nil {
		if text := strings.TrimSpace(string(out)); text != "" {
			return result, fmt.Errorf("写入并重启节点失败: %s", text)
		}
		return result, fmt.Errorf("写入并重启节点失败: %w", err)
	}
	result.Status, result.Detail = "success", strings.TrimSpace(string(out))
	return result, nil
}

func nodeRegistryConfigReadCommand() string {
	return `target="/etc/rancher/k3s/registries.yaml"
if [ "$(id -u)" -eq 0 ]; then
  if [ -f "$target" ]; then cat "$target"; fi
elif sudo -n test -f "$target"; then
  sudo -n cat "$target"
fi`
}

func nodeRegistryConfigWriteCommand(payload string) string {
	return fmt.Sprintf(`set -e
sudo_cmd="sudo -n"
if [ "$(id -u)" -eq 0 ]; then sudo_cmd=""; fi
target="/etc/rancher/k3s/registries.yaml"
unit=""
for candidate in k3s k3s-agent; do
  if [ "$($sudo_cmd systemctl show "${candidate}.service" -p LoadState --value 2>/dev/null)" = "loaded" ]; then unit="${candidate}.service"; break; fi
done
if [ -z "$unit" ]; then echo "未找到 k3s.service 或 k3s-agent.service" >&2; exit 1; fi
backup="${target}.cylism-sync.backup"
tmp="${target}.cylism-sync.tmp"
if $sudo_cmd test -f "$target"; then $sudo_cmd cp "$target" "$backup"; fi
$sudo_cmd mkdir -p /etc/rancher/k3s
echo %q | base64 -d | $sudo_cmd tee "$tmp" >/dev/null
$sudo_cmd chmod 600 "$tmp"
$sudo_cmd mv -f "$tmp" "$target"
restart_failed=0
if ! $sudo_cmd systemctl restart "$unit"; then restart_failed=1; fi
if ! $sudo_cmd systemctl is-active --quiet "$unit"; then restart_failed=1; fi
if [ "$restart_failed" -ne 0 ]; then
  echo "服务重启失败，正在恢复旧配置: $unit" >&2
  if $sudo_cmd test -f "$backup"; then
    $sudo_cmd cp "$backup" "$target"
    $sudo_cmd systemctl restart "$unit" || true
  fi
  exit 1
fi
echo "配置已同步并重启: $unit"`, payload)
}

func mergeSafeRegistryConfigs(raw []byte, configs []SafeRegistryConfig) ([]byte, error) {
	current := k3sRegistriesConfig{Mirrors: map[string]k3sRegistryMirror{}, Configs: map[string]k3sRegistryConfig{}}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := yaml.Unmarshal(raw, &current); err != nil {
			return nil, fmt.Errorf("解析节点 Registry 配置失败")
		}
		if current.Mirrors == nil {
			current.Mirrors = map[string]k3sRegistryMirror{}
		}
		if current.Configs == nil {
			current.Configs = map[string]k3sRegistryConfig{}
		}
	}
	keep := map[string]bool{}
	for _, config := range configs {
		registry := NormalizeRegistryHost(config.Registry)
		if registry == "" {
			continue
		}
		keep[registry] = true
		current.Mirrors[registry] = k3sRegistryMirror{Endpoint: config.Endpoints}
		entry := current.Configs[registry]
		if strings.TrimSpace(config.AuthUsername) != "" || strings.TrimSpace(config.AuthCredential) != "" {
			if entry.Auth == nil {
				entry.Auth = &k3sRegistryAuth{}
			}
			if username := strings.TrimSpace(config.AuthUsername); username != "" {
				entry.Auth.Username = username
			}
			if credential := strings.TrimSpace(config.AuthCredential); credential != "" {
				entry.Auth.Password = credential
				entry.Auth.Token = ""
			}
		}
		if config.InsecureSkipVerify {
			if entry.TLS == nil {
				entry.TLS = &k3sRegistryTLS{}
			}
			entry.TLS.InsecureSkipVerify = true
		} else if entry.TLS != nil {
			entry.TLS.InsecureSkipVerify = false
		}
		current.Configs[registry] = entry
	}
	for registry := range current.Mirrors {
		if !keep[registry] {
			delete(current.Mirrors, registry)
			delete(current.Configs, registry)
		}
	}
	if len(current.Configs) == 0 {
		current.Configs = nil
	}
	if len(current.Mirrors) == 0 {
		current.Mirrors = nil
	}
	return yaml.Marshal(current)
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
