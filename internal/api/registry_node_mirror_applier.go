package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/model"
)

// applyK3sRegistriesToNode is the SSH infrastructure adapter shared by
// Registry workflows. The service supplies already-rendered configuration.
func applyK3sRegistriesToNode(server *model.Server, encKey, content []byte) (string, string) {
	if server.SSHAuthType != "key" || server.SSHKey == "" {
		return "skipped", "需要已配置的 SSH 密钥认证"
	}
	payload := base64.StdEncoding.EncodeToString(content)
	out, err := infrastructureapi.SSHExec(90*time.Second, append(infrastructureapi.BuildSSHArgs(server, encKey, server.Host), nodeRegistryMirrorApplyCommand(payload)))
	if err != nil {
		return "failed", strings.TrimSpace(string(out))
	}
	return "success", "已备份旧配置，K3s 服务重启已安排"
}

func nodeRegistryMirrorApplyCommand(payload string) string {
	return fmt.Sprintf("set -eu; sudo -n mkdir -p /etc/rancher/k3s; if sudo -n test -f /etc/rancher/k3s/registries.yaml; then sudo -n cp /etc/rancher/k3s/registries.yaml /etc/rancher/k3s/registries.yaml.cylism-backup; fi; printf %%s %s | base64 -d | sudo -n tee /etc/rancher/k3s/registries.yaml.tmp >/dev/null; sudo -n chmod 600 /etc/rancher/k3s/registries.yaml.tmp; sudo -n mv /etc/rancher/k3s/registries.yaml.tmp /etc/rancher/k3s/registries.yaml; if sudo -n systemctl is-active --quiet k3s; then service=k3s; elif sudo -n systemctl is-active --quiet k3s-agent; then service=k3s-agent; else echo '未检测到 k3s 或 k3s-agent 服务' >&2; exit 1; fi; sudo -n true; nohup sudo -n sh -c \"sleep 2; systemctl restart $service\" </dev/null >/dev/null 2>&1 &", payload)
}
