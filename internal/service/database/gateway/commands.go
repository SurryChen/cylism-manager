package gateway

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
)

//go:embed proxy.py
var proxySource string

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

const DefaultProbeImage = "python:3.12-alpine"

func Validate(item *model.DatabaseInstance, config *model.DatabaseGateway) error {
	if item.Mode != "external" {
		return fmt.Errorf("私网网关仅适用于已有 MySQL 实例")
	}
	if !hostname.MatchString(item.Host) || strings.Contains(item.Host, "..") || item.Port < 1 || item.Port > 65535 {
		return fmt.Errorf("数据库目标地址或端口无效")
	}
	if ip := net.ParseIP(item.Host); ip != nil && !privateIPv4(ip) {
		return fmt.Errorf("网关目标必须是 RFC1918 私网地址或解析到私网的域名")
	}
	_, tailnet, _ := net.ParseCIDR("100.64.0.0/10")
	ip := net.ParseIP(config.ListenIP)
	if ip == nil || ip.To4() == nil || !tailnet.Contains(ip) {
		return fmt.Errorf("监听地址必须是网关的 Tailscale IPv4 地址（100.64.0.0/10）")
	}
	if config.ListenPort < 1024 || config.ListenPort > 65535 || config.ClusterID == 0 || config.ServerID == 0 {
		return fmt.Errorf("请选择网关服务器、应用集群及 1024–65535 的监听端口")
	}
	if len(config.AllowedSources) == 0 || len(config.AllowedSources) > 64 {
		return fmt.Errorf("请填写 1–64 个允许访问的来源 IPv4/CIDR")
	}
	for index, source := range config.AllowedSources {
		source = strings.TrimSpace(source)
		if ip := net.ParseIP(source); ip != nil && ip.To4() != nil {
			source = ip.String() + "/32"
		}
		_, network, err := net.ParseCIDR(source)
		if err != nil || network.IP.To4() == nil || !(tailnet.Contains(network.IP) || privateIPv4(network.IP)) {
			return fmt.Errorf("来源必须是 Tailscale 或 RFC1918 私网 IPv4/CIDR：%s", source)
		}
		ones, bits := network.Mask.Size()
		last := append(net.IP(nil), network.IP...)
		for i := range last {
			last[i] |= ^network.Mask[i]
		}
		if bits != 32 || ones == 0 || !(tailnet.Contains(last) && tailnet.Contains(network.IP) || privateRange(network.IP, last)) {
			return fmt.Errorf("来源网段不能包含公网地址：%s", source)
		}
		config.AllowedSources[index] = network.String()
	}
	config.ProbeImage = strings.TrimSpace(config.ProbeImage)
	if config.ProbeImage == "" {
		config.ProbeImage = DefaultProbeImage
	}
	if len(config.ProbeImage) > 255 || strings.ContainsAny(config.ProbeImage, " \t\r\n") {
		return fmt.Errorf("诊断镜像地址无效")
	}
	return nil
}

func privateIPv4(ip net.IP) bool { return privateRange(ip, ip) }
func privateRange(first, last net.IP) bool {
	for _, value := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, network, _ := net.ParseCIDR(value)
		if network.Contains(first) && network.Contains(last) {
			return true
		}
	}
	return false
}

func Name(id uint) string        { return fmt.Sprintf("cylism-db-gateway-%d", id) }
func ServiceHost(id uint) string { return Name(id) + ".cylism-databases.svc.cluster.local" }
func quote(value string) string  { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// Select only known system locations; never execute an SSH user's PATH entry as root.
func pythonSelectionCommand(candidates []string) string {
	quoted := make([]string, len(candidates))
	for i, candidate := range candidates {
		quoted[i] = quote(candidate)
	}
	return `gateway_python=
for candidate in ` + strings.Join(quoted, " ") + `; do
  if test -x "$candidate" && "$candidate" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 7) else 1)' >/dev/null 2>&1; then
    gateway_python="$candidate"
    break
  fi
done
test -n "$gateway_python" || { echo '私网网关需要 Python 3.7+；请通过系统官方软件源安装较新 Python（可与系统默认版本并存），然后重新部署'; exit 1; }
`
}

func systemPythonSelectionCommand() string {
	candidates := []string{"/usr/bin/python3"}
	for minor := 15; minor >= 7; minor-- {
		candidates = append(candidates, fmt.Sprintf("/usr/bin/python3.%d", minor))
	}
	return pythonSelectionCommand(candidates)
}

// Commands contain no credentials. Values are passed as base64 or shell-quoted literals.
func DeployCommand(item *model.DatabaseInstance, config *model.DatabaseGateway) string {
	settings, _ := json.Marshal(map[string]any{"target_host": item.Host, "target_port": item.Port, "listen_ip": config.ListenIP, "listen_port": config.ListenPort, "allowed_sources": config.AllowedSources})
	name := Name(config.ID)
	dir := "/etc/cylism/" + name
	unit := "# Managed by Cylism Manager: " + name + "\n[Unit]\nDescription=Cylism private database gateway\nAfter=network-online.target tailscaled.service\nWants=network-online.target\nStartLimitIntervalSec=0\n[Service]\nType=notify\nNotifyAccess=main\nTimeoutStartSec=30\nUser=nobody\nExecStart=" + dir + "/python3 " + dir + "/proxy.py " + dir + "/config.json\nRestart=always\nRestartSec=5\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nProtectKernelTunables=true\nProtectControlGroups=true\nRestrictSUIDSGID=true\nCapabilityBoundingSet=\nRestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX\nMemoryMax=128M\nTasksMax=64\nUMask=0077\n[Install]\nWantedBy=multi-user.target\n"
	return "set -eu\n" + systemPythonSelectionCommand() + `
command -v systemctl >/dev/null
command -v tailscale >/dev/null
test "$(tailscale ip -4)" = ` + quote(config.ListenIP) + ` || { echo '监听地址与网关实际 Tailscale IPv4 不一致'; exit 1; }
exec 9>/run/lock/` + name + `.lock
flock -x 9
dir=` + quote(dir) + `
unit=/etc/systemd/system/` + name + `.service
if test -e "$dir"; then test ! -L "$dir" && test "$(cat "$dir/owner")" = ` + quote(name) + `; fi
if test -e "$unit"; then test ! -L "$unit" && head -n 1 "$unit" | grep -Fx ` + quote("# Managed by Cylism Manager: "+name) + `; fi
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
printf '%s' ` + quote(base64.StdEncoding.EncodeToString([]byte(proxySource))) + ` | base64 -d > "$stage/proxy.py"
printf '%s' ` + quote(base64.StdEncoding.EncodeToString(settings)) + ` | base64 -d > "$stage/config.json"
"$gateway_python" "$stage/proxy.py" "$stage/config.json" --check
mkdir -p "$dir"
chmod 755 "$dir"
printf '%s' ` + quote(name) + ` > "$dir/owner"
ln -sfn "$gateway_python" "$dir/python3"
install -m 644 "$stage/proxy.py" "$dir/proxy.py"
install -m 644 "$stage/config.json" "$dir/config.json"
printf '%s' ` + quote(base64.StdEncoding.EncodeToString([]byte(unit))) + ` | base64 -d > "$unit"
chmod 644 "$unit"
systemctl daemon-reload
systemctl enable ` + quote(name+".service") + `
systemctl restart ` + quote(name+".service") + `
systemctl is-active --quiet ` + quote(name+".service") + `
`
}

func DisableCommand(id uint) string {
	name := Name(id)
	return `set -eu
exec 9>/run/lock/` + name + `.lock
flock -x 9
unit=/etc/systemd/system/` + name + `.service
if test -e "$unit"; then
  test ! -L "$unit"
  head -n 1 "$unit" | grep -Fx ` + quote("# Managed by Cylism Manager: "+name) + `
  systemctl disable --now ` + quote(name+".service") + `
fi
`
}

func Privileged(server *model.Server, command string) string {
	if server.SSHUser == "root" {
		return "sh -c " + quote(command)
	}
	return "sudo -n sh -c " + quote(command)
}

func HealthCommand(config *model.DatabaseGateway) string {
	dir := "/etc/cylism/" + Name(config.ID)
	return "set -eu\n" + "test \"$(tailscale ip -4)\" = " + quote(config.ListenIP) + "\nsystemctl is-active --quiet " + quote(Name(config.ID)+".service") + "\n" +
		"gateway_python=" + quote(dir+"/python3") + "\nif ! test -x \"$gateway_python\"; then\n" + systemPythonSelectionCommand() + "fi\n" +
		"\"$gateway_python\" " + quote(dir+"/proxy.py") + " " + quote(dir+"/config.json") + " --health"
}
