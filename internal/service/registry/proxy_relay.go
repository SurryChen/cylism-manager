package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gopkg.in/yaml.v3"
)

// ProxyRelayService manages only units and config files with the Cylism prefix.
type ProxyRelayService struct{ ssh SSHExecutor }

func NewProxyRelayService(ssh SSHExecutor) *ProxyRelayService { return &ProxyRelayService{ssh: ssh} }

func (s *ProxyRelayService) Deploy(ctx context.Context, server *model.Server, relay *model.RegistryProxyRelay, proxy *model.RegistryProxy) error {
	if s == nil || s.ssh == nil {
		return fmt.Errorf("SSH 执行通道不可用")
	}
	config := fmt.Sprintf("global\n  maxconn 1024\n\ndefaults\n  mode tcp\n  timeout connect 10s\n  timeout client 10m\n  timeout server 10m\n\nfrontend registry\n  bind %s:%d\n  default_backend origin\n\nbackend origin\n  server proxy %s:%d check\n", relay.ListenIP, relay.ListenPort, proxy.EndpointHost, proxy.NodePort)
	payload := base64.StdEncoding.EncodeToString([]byte(config))
	command := fmt.Sprintf(`set -eu
unit="cylism-registry-relay-%d.service"
config="/etc/haproxy/cylism-registry-relay-%d.cfg"
if [ "$(id -u)" -eq 0 ]; then sudo_cmd=""; else sudo_cmd="sudo -n"; fi
if ! ip -o -4 addr show | awk '{print $4}' | cut -d/ -f1 | grep -Fxq '%s'; then echo '监听 IP 不属于目标机器' >&2; exit 1; fi
if ! command -v haproxy >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then $sudo_cmd env DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null && $sudo_cmd env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq haproxy >/dev/null;
  elif command -v dnf >/dev/null 2>&1; then $sudo_cmd dnf install -y -q haproxy >/dev/null;
  else echo '未找到可用的 HAProxy 安装方式' >&2; exit 1; fi
fi
if $sudo_cmd test -e "/etc/systemd/system/$unit"; then
  if ! $sudo_cmd grep -Fxq 'Description=Cylism Registry relay %d' "/etc/systemd/system/$unit"; then echo '同名 systemd 服务不属于平台' >&2; exit 1; fi
  if $sudo_cmd systemctl is-active --quiet "$unit"; then echo '同名中转服务已经运行' >&2; exit 1; fi
fi
if ss -H -ltn "( sport = :%d )" | grep -q .; then echo '监听端口已被占用' >&2; exit 1; fi
if ! curl --noproxy '*' -fsS --connect-timeout 5 --max-time 10 -o /dev/null 'http://%s:%d/v2/'; then echo '目标代理不可达' >&2; exit 1; fi
$sudo_cmd mkdir -p /etc/haproxy
echo '%s' | base64 -d | $sudo_cmd tee "$config" >/dev/null
if ! $sudo_cmd haproxy -c -f "$config"; then $sudo_cmd rm -f "$config"; exit 1; fi
haproxy_bin="$(command -v haproxy)"
printf '%%s\n' '[Unit]' 'Description=Cylism Registry relay %d' 'After=network-online.target' 'Wants=network-online.target' '[Service]' 'Type=simple' "ExecStart=$haproxy_bin -W -db -f $config" 'Restart=on-failure' '[Install]' 'WantedBy=multi-user.target' | $sudo_cmd tee "/etc/systemd/system/$unit" >/dev/null
$sudo_cmd systemctl daemon-reload
if ! $sudo_cmd systemctl enable --now "$unit"; then
  $sudo_cmd journalctl -u "$unit" -n 20 --no-pager -o cat >&2 || true
  $sudo_cmd systemctl disable --now "$unit" || true
  $sudo_cmd rm -f "/etc/systemd/system/$unit" "$config"
  $sudo_cmd systemctl daemon-reload
  echo '中转服务启动失败' >&2
  exit 1
fi
ready=0
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if $sudo_cmd systemctl is-active --quiet "$unit" && curl --noproxy '*' -fsS --connect-timeout 2 --max-time 3 -o /dev/null 'http://%s:%d/v2/' 2>/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo '中转服务未通过健康检查，systemd 日志：' >&2
  $sudo_cmd journalctl -u "$unit" -n 20 --no-pager -o cat >&2 || true
  curl --noproxy '*' -fsS --connect-timeout 2 --max-time 3 -o /dev/null 'http://%s:%d/v2/' || true
  $sudo_cmd systemctl disable --now "$unit" || true
  $sudo_cmd rm -f "/etc/systemd/system/$unit" "$config"
  $sudo_cmd systemctl daemon-reload
  echo '中转服务启动或健康检查失败' >&2
  exit 1
fi`, relay.ID, relay.ID, relay.ListenIP, relay.ID, relay.ListenPort, proxy.EndpointHost, proxy.NodePort, payload, relay.ID, relay.ListenIP, relay.ListenPort, relay.ListenIP, relay.ListenPort)
	out, err := s.ssh.Execute(ctx, 240*time.Second, server, command)
	if err != nil {
		return fmt.Errorf("部署中转失败: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *ProxyRelayService) Check(ctx context.Context, server *model.Server, relay *model.RegistryProxyRelay) error {
	command := fmt.Sprintf(`set -eu
if [ "$(id -u)" -eq 0 ]; then sudo_cmd=""; else sudo_cmd="sudo -n"; fi
$sudo_cmd systemctl is-active --quiet cylism-registry-relay-%d.service
curl --noproxy '*' -fsS --connect-timeout 5 --max-time 10 -o /dev/null 'http://%s:%d/v2/'`, relay.ID, relay.ListenIP, relay.ListenPort)
	out, err := s.ssh.Execute(ctx, 20*time.Second, server, command)
	if err != nil {
		return fmt.Errorf("中转不可用: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *ProxyRelayService) Remove(ctx context.Context, server *model.Server, relay *model.RegistryProxyRelay) error {
	command := fmt.Sprintf(`set -eu
if [ "$(id -u)" -eq 0 ]; then sudo_cmd=""; else sudo_cmd="sudo -n"; fi
unit="cylism-registry-relay-%d.service"
config="/etc/haproxy/cylism-registry-relay-%d.cfg"
if $sudo_cmd test -f "/etc/systemd/system/$unit"; then
  if ! $sudo_cmd grep -Fq 'Description=Cylism Registry relay %d' "/etc/systemd/system/$unit"; then echo '服务不是平台创建的' >&2; exit 1; fi
  $sudo_cmd systemctl disable --now "$unit"
  $sudo_cmd rm -f "/etc/systemd/system/$unit" "$config"
  $sudo_cmd systemctl daemon-reload
fi`, relay.ID, relay.ID, relay.ID)
	out, err := s.ssh.Execute(ctx, 30*time.Second, server, command)
	if err != nil {
		return fmt.Errorf("移除中转失败: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SwitchNode replaces one mirror endpoint while retaining all other YAML keys,
// including registry authentication. The existing writer backs up the file and
// restores it if K3s fails to restart.
func (s *ProxyRelayService) SwitchNode(ctx context.Context, server *model.Server, registry, endpoint string) error {
	if s == nil || s.ssh == nil {
		return fmt.Errorf("SSH 执行通道不可用")
	}
	command := fmt.Sprintf("curl --noproxy '*' -fsS --connect-timeout 5 --max-time 10 -o /dev/null '%s/v2/'", endpoint)
	if out, err := s.ssh.Execute(ctx, 20*time.Second, server, command); err != nil {
		return fmt.Errorf("节点无法访问目标入口: %s: %w", strings.TrimSpace(string(out)), err)
	}
	raw, err := s.ssh.Execute(ctx, 30*time.Second, server, strictNodeRegistryConfigReadCommand())
	if err != nil {
		return fmt.Errorf("读取节点镜像源配置失败: %w", err)
	}
	updated, err := setRegistryEndpoint(raw, registry, endpoint)
	if err != nil {
		return err
	}
	writer := NewNodeMirrorApplier(s.ssh)
	result, err := writer.writeToTarget(ctx, server, updated)
	if err != nil {
		return err
	}
	if result.Status != "success" {
		return fmt.Errorf("节点镜像源应用失败: %s", result.Detail)
	}
	return nil
}

func (s *ProxyRelayService) CheckNodeRoute(ctx context.Context, server *model.Server, registry, endpoint string) error {
	raw, err := s.ssh.Execute(ctx, 30*time.Second, server, strictNodeRegistryConfigReadCommand())
	if err != nil {
		return fmt.Errorf("读取节点镜像源配置失败: %w", err)
	}
	configs, err := parseSafeRegistryConfigs(raw)
	if err != nil {
		return fmt.Errorf("节点镜像源配置格式无效: %w", err)
	}
	for _, config := range configs {
		if config.Registry != registry {
			continue
		}
		if len(config.Endpoints) != 1 || config.Endpoints[0] != endpoint {
			return fmt.Errorf("节点当前入口与平台记录不一致")
		}
		command := fmt.Sprintf("curl --noproxy '*' -fsS --connect-timeout 5 --max-time 10 -o /dev/null '%s/v2/'", endpoint)
		out, err := s.ssh.Execute(ctx, 20*time.Second, server, command)
		if err != nil {
			return fmt.Errorf("节点访问入口失败: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	return fmt.Errorf("节点未配置此 Registry 的镜像入口")
}

func (s *ProxyRelayService) ConfirmNodeRouteChanged(ctx context.Context, server *model.Server, registry, endpoint string) error {
	used, err := s.NodeUsesEndpoint(ctx, server, registry, endpoint)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("节点仍在使用此入口，请先在镜像源配置中切换")
	}
	return nil
}

func (s *ProxyRelayService) NodeUsesEndpoint(ctx context.Context, server *model.Server, registry, endpoint string) (bool, error) {
	if s == nil || s.ssh == nil {
		return false, fmt.Errorf("SSH 执行通道不可用")
	}
	raw, err := s.ssh.Execute(ctx, 30*time.Second, server, strictNodeRegistryConfigReadCommand())
	if err != nil {
		return false, fmt.Errorf("读取节点镜像源配置失败: %w", err)
	}
	configs, err := parseSafeRegistryConfigs(raw)
	if err != nil {
		return false, fmt.Errorf("节点镜像源配置格式无效: %w", err)
	}
	for _, config := range configs {
		if config.Registry != registry {
			continue
		}
		for _, actual := range config.Endpoints {
			if actual == endpoint {
				return true, nil
			}
		}
	}
	return false, nil
}

func setRegistryEndpoint(raw []byte, registry, endpoint string) ([]byte, error) {
	var document yaml.Node
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := yaml.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("节点镜像源配置格式无效: %w", err)
		}
	}
	if len(document.Content) == 0 {
		document.Kind = yaml.DocumentNode
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("节点镜像源配置格式无效")
	}
	mirrors := mappingValue(root, "mirrors")
	if mirrors == nil {
		mirrors = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "mirrors"}, mirrors)
	}
	if mirrors.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("节点 mirrors 配置格式无效")
	}
	entry := mappingValue(mirrors, registry)
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mirrors.Content = append(mirrors.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: registry}, entry)
	}
	if entry.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("节点 registry 配置格式无效")
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: endpoint}}}
	for i := 0; i+1 < len(entry.Content); i += 2 {
		if entry.Content[i].Value == "endpoint" {
			entry.Content[i+1] = sequence
			return yaml.Marshal(&document)
		}
	}
	entry.Content = append(entry.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "endpoint"}, sequence)
	return yaml.Marshal(&document)
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func strictNodeRegistryConfigReadCommand() string {
	return `set -e
target="/etc/rancher/k3s/registries.yaml"
if [ "$(id -u)" -eq 0 ]; then
  if [ -f "$target" ]; then cat "$target"; fi
else
  sudo -n true
  if sudo -n test -f "$target"; then sudo -n cat "$target"; fi
fi`
}
