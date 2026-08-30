package api

import (
	"fmt"
	"strings"
	"time"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	"github.com/cylism/cylism-manager/internal/model"
)

type agentMaintenanceCleanupExecutor func(*model.Server, string) (string, error)

// defaultAgentMaintenanceCleanupExecutor intentionally accepts a recipe ID,
// not a command. These immutable scripts are the only remote mutations this
// capability can ever execute.
func defaultAgentMaintenanceCleanupExecutor(encKey []byte) agentMaintenanceCleanupExecutor {
	return func(server *model.Server, recipe string) (string, error) {
		if server == nil || !validMaintenanceRecipe(recipe) {
			return "", fmt.Errorf("node or cleanup recipe unavailable")
		}
		command := ""
		switch recipe {
		case "journal-vacuum":
			command = "sudo -n journalctl --disk-usage; sudo -n journalctl --vacuum-time=7d; sudo -n journalctl --disk-usage"
		case "container-image-prune":
			command = containerImagePruneCommand()
		case "docker-image-prune":
			command = dockerImagePruneCommand()
		}
		args := infrastructureapi.BuildSSHArgs(server, encKey, server.Host)
		args = append(args, command)
		output, err := infrastructureapi.SSHExec(2*time.Minute, args)
		summary := truncateAgentText(strings.TrimSpace(redactAgentText(string(output))), agentOperationErrorSummaryLimit)
		if err != nil {
			if summary == "" {
				return "", fmt.Errorf("maintenance cleanup failed: %w", err)
			}
			return summary, fmt.Errorf("maintenance cleanup failed: %w", err)
		}
		return summary, nil
	}
}

func containerImagePruneCommand() string {
	return `set -eu; if [ -x /usr/local/bin/crictl ]; then sudo -n /usr/local/bin/crictl images; sudo -n /usr/local/bin/crictl rmi --prune; sudo -n /usr/local/bin/crictl images; elif [ -x /var/lib/rancher/k3s/bin/crictl ]; then sudo -n /var/lib/rancher/k3s/bin/crictl images; sudo -n /var/lib/rancher/k3s/bin/crictl rmi --prune; sudo -n /var/lib/rancher/k3s/bin/crictl images; elif [ -x /usr/local/bin/k3s ]; then sudo -n /usr/local/bin/k3s crictl images; sudo -n /usr/local/bin/k3s crictl rmi --prune; sudo -n /usr/local/bin/k3s crictl images; else exit 127; fi`
}

func dockerImagePruneCommand() string {
	return `set -eu; if [ -x /usr/bin/docker ]; then sudo -n /usr/bin/docker system df; sudo -n /usr/bin/docker image prune -af; sudo -n /usr/bin/docker system df; elif [ -x /usr/local/bin/docker ]; then sudo -n /usr/local/bin/docker system df; sudo -n /usr/local/bin/docker image prune -af; sudo -n /usr/local/bin/docker system df; else exit 127; fi`
}

func maintenanceCompletionSummary(recipe, output string) string {
	prefix := "固定清理配方执行完成"
	if recipe == "journal-vacuum" {
		prefix = "系统日志清理完成"
	}
	if recipe == "container-image-prune" {
		prefix = "未使用容器镜像清理完成"
	}
	if recipe == "docker-image-prune" {
		prefix = "未使用 Docker 镜像清理完成"
	}
	if output == "" {
		return prefix
	}
	return truncateAgentText(prefix+": "+output, 512)
}
