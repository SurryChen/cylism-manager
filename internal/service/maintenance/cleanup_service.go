package maintenance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

const cleanupErrorLimit = 512

type CleanupService struct {
	ssh SSHExecutor
}

func NewCleanupService(ssh SSHExecutor) *CleanupService { return &CleanupService{ssh: ssh} }

func (s *CleanupService) Execute(ctx context.Context, server *model.Server, recipe string) (string, error) {
	if server == nil || !ValidCleanupRecipe(recipe) {
		return "", fmt.Errorf("node or cleanup recipe unavailable")
	}
	if s == nil || s.ssh == nil {
		return "", fmt.Errorf("maintenance cleanup unavailable")
	}
	command := cleanupCommand(recipe)
	output, err := s.ssh.Execute(ctx, 2*time.Minute, server, command)
	summary := cleanOutput(string(output), cleanupErrorLimit)
	if err != nil {
		return summary, fmt.Errorf("maintenance cleanup failed: %w", err)
	}
	return summary, nil
}

func CleanupCompletionSummary(recipe, output string) string {
	prefix := "固定清理配方执行完成"
	switch recipe {
	case "journal-vacuum":
		prefix = "系统日志清理完成"
	case "container-image-prune":
		prefix = "未使用容器镜像清理完成"
	case "docker-image-prune":
		prefix = "未使用 Docker 镜像清理完成"
	}
	if strings.TrimSpace(output) == "" {
		return prefix
	}
	return truncate(prefix+": "+output, 512)
}

func cleanupCommand(recipe string) string {
	switch recipe {
	case "journal-vacuum":
		return "sudo -n journalctl --disk-usage; sudo -n journalctl --vacuum-time=7d; sudo -n journalctl --disk-usage"
	case "container-image-prune":
		return containerImagePruneCommand()
	case "docker-image-prune":
		return dockerImagePruneCommand()
	default:
		return ""
	}
}

func ContainerImagePruneCommand() string {
	return containerImagePruneCommand()
}

func DockerImagePruneCommand() string { return dockerImagePruneCommand() }

func containerImagePruneCommand() string {
	return `set -eu; if [ -x /usr/local/bin/crictl ]; then sudo -n /usr/local/bin/crictl images; sudo -n /usr/local/bin/crictl rmi --prune; sudo -n /usr/local/bin/crictl images; elif [ -x /var/lib/rancher/k3s/bin/crictl ]; then sudo -n /var/lib/rancher/k3s/bin/crictl images; sudo -n /var/lib/rancher/k3s/bin/crictl rmi --prune; sudo -n /var/lib/rancher/k3s/bin/crictl images; elif [ -x /usr/local/bin/k3s ]; then sudo -n /usr/local/bin/k3s crictl images; sudo -n /usr/local/bin/k3s crictl rmi --prune; sudo -n /usr/local/bin/k3s crictl images; else exit 127; fi`
}

func dockerImagePruneCommand() string {
	return `set -eu; if [ -x /usr/bin/docker ]; then sudo -n /usr/bin/docker system df; sudo -n /usr/bin/docker image prune -af; sudo -n /usr/bin/docker system df; elif [ -x /usr/local/bin/docker ]; then sudo -n /usr/local/bin/docker system df; sudo -n /usr/local/bin/docker image prune -af; sudo -n /usr/local/bin/docker system df; else exit 127; fi`
}
