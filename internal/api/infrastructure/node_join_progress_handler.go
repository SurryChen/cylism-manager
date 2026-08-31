package infrastructure

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/gin-gonic/gin"
)

// NodeJoinProgressHandler streams the existing worker installation workflow.
// It is kept as a transport adapter; cluster lifecycle decisions stay in the
// cluster service and the SSH operations use the shared infrastructure client.
type NodeJoinProgressHandler struct {
	store  repository.NodeJoinRepository
	encKey []byte
	k8s    *k8s.Client
}

const nodeJoinSSHTimeout = 15 * time.Second

func NewNodeJoinProgressHandler(st repository.NodeJoinRepository, encKey []byte, client *k8s.Client) *NodeJoinProgressHandler {
	return &NodeJoinProgressHandler{store: st, encKey: encKey, k8s: client}
}

func (h *NodeJoinProgressHandler) JoinProgress(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, 404, model.CodeNotFound, "server not found")
		return
	}
	conn, err := wsUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	clientClosed := make(chan struct{})
	go func() { _ = conn.readClose(); close(clientClosed) }()
	go func() {
		defer conn.Close()
		total, index := 12, 0
		now := func() string { return time.Now().Format(time.RFC3339) }
		aborted := func() bool {
			select {
			case <-clientClosed:
				return true
			default:
				return false
			}
		}
		send := func(step, label, status, detail string) bool {
			if aborted() {
				return false
			}
			return sendProgress(conn, total, index, step, label, status, detail, now())
		}
		fail := func(step, label, detail string) { index++; _ = send(step, label, wsStatusFailed, detail) }
		sshArgs := buildSSHArgs(server, h.encKey, server.Host)
		checks := []struct{ name, label, command string }{
			{"ssh_connect", "SSH 连接", "echo ok"},
			{"root_privilege", "Root 权限", "id -u"},
			{"swap_disabled", "Swap 状态", "swapon --show 2>/dev/null | wc -l"},
			{"os_compatible", "操作系统", "uname -m"},
			{"disk_space", "磁盘空间", "df -BG / | tail -1 | awk '{print $4}' | sed 's/G//'"},
		}
		for _, check := range checks {
			if aborted() {
				return
			}
			index++
			if !send(check.name, check.label, wsStatusRunning, "检测中...") {
				return
			}
			out, runErr := sshExec(nodeJoinSSHTimeout, append(sshArgs, check.command))
			result := strings.TrimSpace(string(out))
			if runErr != nil {
				fail(check.name, check.label, fmt.Sprintf("失败: %v", runErr))
				return
			}
			pass := (check.name == "ssh_connect" && strings.Contains(result, "ok")) ||
				(check.name == "root_privilege" && result == "0") ||
				(check.name == "swap_disabled" && result == "0") ||
				(check.name == "os_compatible" && (strings.Contains(result, "x86_64") || strings.Contains(result, "aarch64")))
			if check.name == "disk_space" {
				var gb int
				_, _ = fmt.Sscanf(result, "%d", &gb)
				pass = gb >= 2
				result = fmt.Sprintf("可用 %dGB", gb)
			}
			if !pass {
				fail(check.name, check.label, result)
				return
			}
			if !send(check.name, check.label, wsStatusSuccess, result) {
				return
			}
		}

		index++
		if !send("check_tailscale", "检测 Tailscale", wsStatusRunning, "检测中...") {
			return
		}
		if _, lookupErr := sshExec(20*time.Second, append(sshArgs, "which tailscale")); lookupErr != nil {
			if !send("check_tailscale", "安装 Tailscale", wsStatusRunning, "未安装，正在安装...") {
				return
			}
			if out, installErr := sshExec(120*time.Second, append(sshArgs, "curl -fsSL https://tailscale.com/install.sh | sh")); installErr != nil {
				fail("check_tailscale", "安装 Tailscale", fmt.Sprintf("失败: %v — %s", installErr, out))
				return
			}
		}
		if !send("check_tailscale", "检测 Tailscale", wsStatusSuccess, "已就绪") {
			return
		}

		index++
		if !send("register_tailscale", "注册 Tailscale", wsStatusRunning, "正在注册...") {
			return
		}
		authKey, _ := h.store.GetSystemConfig("tailscale_auth_key")
		if authKey == "" {
			fail("register_tailscale", "注册 Tailscale", "Auth Key 未配置")
			return
		}
		if out, upErr := sshExec(60*time.Second, append(sshArgs, "tailscale", "up", "--reset", "--auth-key="+authKey, "--accept-routes")); upErr != nil {
			fail("register_tailscale", "注册 Tailscale", fmt.Sprintf("失败: %v — %s", upErr, out))
			return
		}
		if !send("register_tailscale", "注册 Tailscale", wsStatusSuccess, "已注册") {
			return
		}

		index++
		if !send("install_k3s_agent", "安装 k3s-agent", wsStatusRunning, "正在安装...") {
			return
		}
		token, _ := h.store.GetSystemConfig("k3s_join_token")
		var controlIP string
		servers, _ := h.store.ListServers()
		for _, candidate := range servers {
			if candidate.ClusterRole == "control-plane" && candidate.Host != "" {
				controlIP = candidate.Host
				break
			}
		}
		if token == "" || controlIP == "" {
			fail("install_k3s_agent", "安装 k3s-agent", "Token/IP 不可用")
			return
		}
		install := fmt.Sprintf(`curl -sfL https://get.k3s.io | K3S_URL='https://%s:6443' K3S_TOKEN='%s' sh -`, shellEscape(controlIP), shellEscape(token))
		if out, installErr := sshExec(180*time.Second, append(sshArgs, install)); installErr != nil {
			fail("install_k3s_agent", "安装 k3s-agent", fmt.Sprintf("失败: %v — %s", installErr, out))
			return
		}
		if !send("install_k3s_agent", "安装 k3s-agent", wsStatusSuccess, "已安装") {
			return
		}

		index++
		if !send("start_service", "启动服务", wsStatusRunning, "等待 k3s-agent 启动...") {
			return
		}
		if out, serviceErr := sshExec(30*time.Second, append(sshArgs, "systemctl is-active k3s-agent")); serviceErr != nil {
			fail("start_service", "启动服务", fmt.Sprintf("失败: %s", out))
			return
		}
		if !send("start_service", "启动服务", wsStatusSuccess, "已启动") {
			return
		}
		index++
		if !send("wait_ready", "等待节点就绪", wsStatusRunning, "等待注册到集群...") {
			return
		}
		ready := false
		for i := 0; i < 60 && !aborted(); i++ {
			time.Sleep(5 * time.Second)
			if h.k8s != nil {
				node, getErr := h.k8s.GetNodeInfo(server.K8sNodeName)
				if getErr == nil && node != nil && node.Ready {
					ready = true
					break
				}
			}
		}
		if !ready {
			fail("wait_ready", "等待节点就绪", "超时 (300s)")
			return
		}
		if !send("wait_ready", "等待节点就绪", wsStatusSuccess, "节点已就绪") {
			return
		}
		server.ClusterRole = "worker"
		if hostname, hostErr := sshExec(10*time.Second, append(sshArgs, "hostname")); hostErr == nil {
			server.K8sNodeName = strings.TrimSpace(string(hostname))
		}
		_ = h.store.UpdateServer(server)
		index++
		_ = send("complete", "加入完成", wsStatusSuccess, fmt.Sprintf("节点 %s 已加入集群", server.Name))
	}()
}

func shellEscape(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
