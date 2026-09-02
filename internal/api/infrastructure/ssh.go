package infrastructure

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/cluster"
	"github.com/gin-gonic/gin"
)

// SSHTimeout is the default bound for a single remote SSH command.
const SSHTimeout = 15 * time.Second

const sshTimeout = SSHTimeout

func sshExec(ctx context.Context, timeout time.Duration, args []string) ([]byte, error) {
	return SSHExecContext(ctx, timeout, args)
}

func buildSSHArgs(server *model.Server, encKey []byte, host string) []string {
	return BuildSSHArgs(server, encKey, host)
}

// SSHExecContext executes a remote command with the caller's cancellation
// boundary and a per-command timeout.
func SSHExecContext(parent context.Context, timeout time.Duration, args []string) ([]byte, error) {
	if parent == nil {
		return nil, fmt.Errorf("ssh context is required")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	log.Printf("[ssh] running remote command for host %s", sshTarget(args))
	return exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
}

// BuildSSHArgs constructs non-interactive SSH arguments. Private keys are
// decrypted only for the duration needed to write a mode-0600 temporary file
// under the application data volume; the key material is never logged.
func BuildSSHArgs(server *model.Server, encKey []byte, host string) []string {
	port := 22
	if server != nil && server.SSHPort > 0 {
		port = server.SSHPort
	}
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-p", strconv.Itoa(port),
	}
	if server != nil && server.SSHAuthType == "key" && server.SSHKey != "" {
		if decKey, err := crypto.Decrypt(encKey, server.SSHKey); err == nil && decKey != "" {
			if server.SSHKeyHash != "" {
				got := md5.Sum([]byte(decKey))
				if hex.EncodeToString(got[:]) != server.SSHKeyHash {
					log.Printf("[ssh] private key hash mismatch for server=%d", server.ID)
				}
			}
			if !strings.HasSuffix(decKey, "\n") {
				decKey += "\n"
			}
			keyDir := filepath.Join("/data", "tmp")
			if err := os.MkdirAll(keyDir, 0700); err == nil {
				keyPath := filepath.Join(keyDir, fmt.Sprintf("cylism-ssh-%d", server.ID))
				if err := os.WriteFile(keyPath, []byte(decKey), 0600); err == nil {
					args = append(args, "-i", keyPath)
				} else {
					log.Printf("[ssh] private key file unavailable for server=%d: %v", server.ID, err)
				}
			}
		} else {
			log.Printf("[ssh] private key decryption failed for server=%d", server.ID)
		}
	}
	if server != nil {
		args = append(args, fmt.Sprintf("%s@%s", server.SSHUser, host))
	} else {
		args = append(args, host)
	}
	return args
}

// ProbeSSH checks that a server accepts a non-interactive SSH command.
func ProbeSSHContext(ctx context.Context, server *model.Server, encKey []byte) (bool, string) {
	if server == nil {
		return false, "server unavailable"
	}
	out, err := SSHExecContext(ctx, SSHTimeout, append(BuildSSHArgs(server, encKey, server.Host), "echo ok"))
	if err != nil {
		return false, fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok" {
		return true, ""
	}
	return false, "SSH probe returned an unexpected response"
}

// RunPrechecks performs the standard server checks used by both probe and
// node-import workflows. It returns the transport-independent Cluster shape.
func RunPrechecksContext(ctx context.Context, server *model.Server, encKey []byte) []cluster.Precheck {
	args := BuildSSHArgs(server, encKey, server.Host)
	checks := make([]cluster.Precheck, 0, 5)
	out, err := SSHExecContext(ctx, SSHTimeout, append(args, "echo ok"))
	sshOK := err == nil && (strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok")
	checks = append(checks, cluster.Precheck{Name: "ssh_connect", Label: "SSH 连接", Pass: sshOK, Detail: precheckDetail(sshOK, "连接成功", fmt.Sprintf("连接失败: %v", err))})
	if !sshOK {
		for _, item := range []struct{ name, label string }{{"root_privilege", "Root 权限"}, {"swap_disabled", "Swap 状态"}, {"os_compatible", "操作系统"}, {"disk_space", "磁盘空间"}} {
			checks = append(checks, cluster.Precheck{Name: item.name, Label: item.label, Detail: "SSH 不可达，跳过"})
		}
		return checks
	}
	out, err = SSHExecContext(ctx, SSHTimeout, append(args, "id -u"))
	rootOK := err == nil && strings.TrimSpace(string(out)) == "0"
	rootDetail := strings.TrimSpace(string(out))
	if !rootOK {
		sudoOut, sudoErr := SSHExecContext(ctx, SSHTimeout, append(args, "sudo -n true 2>&1"))
		if sudoErr == nil || strings.Contains(string(sudoOut), "password") {
			rootOK, rootDetail = true, "有 sudo 权限（非 root 用户）"
		} else {
			rootDetail = fmt.Sprintf("无 root 权限且无 sudo: %s", strings.TrimSpace(string(sudoOut)))
		}
	}
	checks = append(checks, cluster.Precheck{Name: "root_privilege", Label: "Root 权限", Pass: rootOK, Detail: rootDetail})
	out, err = SSHExecContext(ctx, SSHTimeout, append(args, "swapon --show 2>/dev/null | wc -l"))
	swapOK := err == nil && strings.TrimSpace(string(out)) == "0"
	checks = append(checks, cluster.Precheck{Name: "swap_disabled", Label: "Swap 状态", Pass: swapOK, Detail: precheckDetail(swapOK, "已关闭", "swap 已开启")})
	out, err = SSHExecContext(ctx, SSHTimeout, append(args, "uname -m"))
	osOK := err == nil && (strings.Contains(string(out), "x86_64") || strings.Contains(string(out), "aarch64"))
	checks = append(checks, cluster.Precheck{Name: "os_compatible", Label: "操作系统", Pass: osOK, Detail: strings.TrimSpace(string(out))})
	out, err = SSHExecContext(ctx, SSHTimeout, append(args, "df -BG / | tail -1 | awk '{print $4}' | sed 's/G//'"))
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

// ServerInspector adapts the shared SSH checks to Cluster Service interfaces.
type ServerInspector struct{ EncKey []byte }

func (i ServerInspector) Probe(ctx context.Context, server *model.Server) (bool, string) {
	return ProbeSSHContext(ctx, server, i.EncKey)
}
func (i ServerInspector) Precheck(ctx context.Context, server *model.Server) []cluster.Precheck {
	return RunPrechecksContext(ctx, server, i.EncKey)
}
func (i ServerInspector) Hostname(ctx context.Context, server *model.Server) (string, error) {
	if _, err := SSHExecContext(ctx, SSHTimeout, append(BuildSSHArgs(server, i.EncKey, server.Host), "echo ok")); err != nil {
		return "", err
	}
	out, err := SSHExecContext(ctx, SSHTimeout, append(BuildSSHArgs(server, i.EncKey, server.Host), "hostname"))
	if err != nil {
		return "", err
	}
	hostname := CleanHostname(string(out))
	if hostname == "" {
		return "", fmt.Errorf("主机名为空")
	}
	return hostname, nil
}

// ServerMetricsInspector adapts the shared resource command to Cluster Service.
type ServerMetricsInspector struct{ EncKey []byte }

func (i ServerMetricsInspector) ResourceStats(ctx context.Context, server *model.Server) (map[string]interface{}, error) {
	command := "echo 'CPU:' $(top -bn1 | awk '/^%Cpu|^CPU:/{print 100-$8}');echo 'CPU_CORES:' $(nproc);echo 'MEM:' $(free -m | awk '/^Mem:/{print $2,$3,$7}');echo 'DISK:' $(df -BG / | awk 'NR==2{print $2,$3,$4,$5}' | sed 's/G//g');echo 'LOAD:' $(cat /proc/loadavg | awk '{print $1,$2,$3}');echo 'UP:' $(uptime -p | sed 's/up //')"
	out, err := SSHExecContext(ctx, 5*time.Second, append(BuildSSHArgs(server, i.EncKey, server.Host), command))
	if err != nil {
		return nil, err
	}
	result := ParseServerStats(string(out))
	parsed := make(map[string]interface{}, len(result))
	for key, value := range result {
		parsed[key] = value
	}
	return parsed, nil
}

// ParseServerStats parses the stable key/value protocol emitted by the stats command.
func ParseServerStats(raw string) gin.H {
	result := gin.H{}
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "CPU:":
			result["cpu_percent"], _ = strconv.ParseFloat(parts[1], 64)
		case "CPU_CORES:":
			result["cpu_cores"], _ = strconv.Atoi(parts[1])
		case "MEM:":
			if len(parts) >= 4 {
				result["memory_total_mb"], _ = strconv.Atoi(parts[1])
				result["memory_used_mb"], _ = strconv.Atoi(parts[2])
				result["memory_available_mb"], _ = strconv.Atoi(parts[3])
			}
		case "DISK:":
			if len(parts) >= 4 {
				result["disk_total_gb"], _ = strconv.Atoi(parts[1])
				result["disk_used_gb"], _ = strconv.Atoi(parts[2])
				result["disk_available_gb"], _ = strconv.Atoi(parts[3])
				if len(parts) >= 5 {
					result["disk_percent"] = parts[4]
				}
			}
		case "LOAD:":
			if len(parts) >= 4 {
				result["load_1m"], _ = strconv.ParseFloat(parts[1], 64)
				result["load_5m"], _ = strconv.ParseFloat(parts[2], 64)
				result["load_15m"], _ = strconv.ParseFloat(parts[3], 64)
			}
		case "UP:":
			result["uptime"] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "UP:"))
		}
	}
	return result
}

// CleanHostname strips SSH warning lines and returns the first useful line.
func CleanHostname(raw string) string {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Warning:") || strings.Contains(line, "Permanently") {
			continue
		}
		return line
	}
	return ""
}

func sshTarget(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		if strings.Contains(args[index], "@") {
			return args[index]
		}
	}
	return "unknown"
}
