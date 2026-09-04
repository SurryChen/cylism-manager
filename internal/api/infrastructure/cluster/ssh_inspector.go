package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/cylism/cylism-manager/internal/transport"
)

// ProbeSSHContext checks that a server accepts a non-interactive SSH command.
func ProbeSSHContext(ctx context.Context, server *model.Server, encKey []byte) (bool, string) {
	if server == nil {
		return false, "server unavailable"
	}
	out, err := transport.SSHExecContext(ctx, transport.SSHTimeout, append(transport.BuildSSHArgs(server, encKey, server.Host), "echo ok"))
	if err != nil {
		return false, fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok" {
		return true, ""
	}
	return false, "SSH probe returned an unexpected response"
}

// RunPrechecksContext performs standard server checks used by probe/import workflows.
func RunPrechecksContext(ctx context.Context, server *model.Server, encKey []byte) []cluster.Precheck {
	args := transport.BuildSSHArgs(server, encKey, server.Host)
	checks := make([]cluster.Precheck, 0, 5)
	out, err := transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "echo ok"))
	sshOK := err == nil && (strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok")
	checks = append(checks, cluster.Precheck{Name: "ssh_connect", Label: "SSH 连接", Pass: sshOK, Detail: precheckDetail(sshOK, "连接成功", fmt.Sprintf("连接失败: %v", err))})
	if !sshOK {
		for _, item := range []struct{ name, label string }{{"root_privilege", "Root 权限"}, {"swap_disabled", "Swap 状态"}, {"os_compatible", "操作系统"}, {"disk_space", "磁盘空间"}} {
			checks = append(checks, cluster.Precheck{Name: item.name, Label: item.label, Detail: "SSH 不可达，跳过"})
		}
		return checks
	}
	out, err = transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "id -u"))
	rootOK := err == nil && strings.TrimSpace(string(out)) == "0"
	rootDetail := strings.TrimSpace(string(out))
	if !rootOK {
		sudoOut, sudoErr := transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "sudo -n true 2>&1"))
		if sudoErr == nil || strings.Contains(string(sudoOut), "password") {
			rootOK, rootDetail = true, "有 sudo 权限（非 root 用户）"
		} else {
			rootDetail = fmt.Sprintf("无 root 权限且无 sudo: %s", strings.TrimSpace(string(sudoOut)))
		}
	}
	checks = append(checks, cluster.Precheck{Name: "root_privilege", Label: "Root 权限", Pass: rootOK, Detail: rootDetail})
	out, err = transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "swapon --show 2>/dev/null | wc -l"))
	swapOK := err == nil && strings.TrimSpace(string(out)) == "0"
	checks = append(checks, cluster.Precheck{Name: "swap_disabled", Label: "Swap 状态", Pass: swapOK, Detail: precheckDetail(swapOK, "已关闭", "swap 已开启")})
	out, err = transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "uname -m"))
	osOK := err == nil && (strings.Contains(string(out), "x86_64") || strings.Contains(string(out), "aarch64"))
	checks = append(checks, cluster.Precheck{Name: "os_compatible", Label: "操作系统", Pass: osOK, Detail: strings.TrimSpace(string(out))})
	out, err = transport.SSHExecContext(ctx, transport.SSHTimeout, append(args, "df -BG / | tail -1 | awk '{print $4}' | sed 's/G//'"))
	diskGB := 0
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &diskGB)
	}
	checks = append(checks, cluster.Precheck{Name: "disk_space", Label: "磁盘空间", Pass: diskGB >= 2, Detail: fmt.Sprintf("可用 %dGB", diskGB)})
	return checks
}

func precheckDetail(pass bool, success, failure string) string {
	if pass {
		return success
	}
	return failure
}

// ServerInspector adapts SSH checks to Cluster Service interfaces.
type ServerInspector struct{ EncKey []byte }

func (i ServerInspector) Probe(ctx context.Context, server *model.Server) (bool, string) {
	return ProbeSSHContext(ctx, server, i.EncKey)
}
func (i ServerInspector) Precheck(ctx context.Context, server *model.Server) []cluster.Precheck {
	return RunPrechecksContext(ctx, server, i.EncKey)
}
func (i ServerInspector) Hostname(ctx context.Context, server *model.Server) (string, error) {
	if _, err := transport.SSHExecContext(ctx, transport.SSHTimeout, append(transport.BuildSSHArgs(server, i.EncKey, server.Host), "echo ok")); err != nil {
		return "", err
	}
	out, err := transport.SSHExecContext(ctx, transport.SSHTimeout, append(transport.BuildSSHArgs(server, i.EncKey, server.Host), "hostname"))
	if err != nil {
		return "", err
	}
	hostname := CleanHostname(string(out))
	if hostname == "" {
		return "", fmt.Errorf("主机名为空")
	}
	return hostname, nil
}
