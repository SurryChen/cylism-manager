package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// NodeHandler K8s 节点管理的 HTTP handler
type NodeHandler struct {
	store *store.Store
}

// NewNodeHandler 创建 NodeHandler
func NewNodeHandler(s *store.Store) *NodeHandler {
	return &NodeHandler{store: s}
}

// ListNode 列出所有集群节点
func (h *NodeHandler) ListNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	nodes, err := K8s.ListNodeInfos()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, nodes)
}

// AddNode 将已注册的服务器加入 K3s 集群
func (h *NodeHandler) AddNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.SuccessWithMessage(c, gin.H{"server": server.Name}, "加入集群 - SSH 集成待实现")
}

// DrainNode 驱逐节点
func (h *NodeHandler) DrainNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	id := c.Param("id")
	if err := K8s.DrainNode(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "驱逐成功")
}

// RemoveNode 从集群移除节点
func (h *NodeHandler) RemoveNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	id := c.Param("id")
	if err := K8s.DeleteNode(id); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "移除成功")
}


// JoinProgress WebSocket 加入集群进度 /api/nodes/:id/join-progress
func (h *NodeHandler) JoinProgress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}

	conn, err := wsUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}

	// Read client close in background
	clientClosed := make(chan struct{})
	go func() {
		conn.readClose()
		close(clientClosed)
	}()

	go func() {
		defer conn.Close()

		host := server.SSHHost
		if host == "" {
			host = server.Host
		}

		total := 12
		idx := 0
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
			return sendProgress(conn, total, idx, step, label, status, detail, now())
		}
		fail := func(step, label, detail string) {
			idx++
			send(step, label, wsStatusFailed, detail)
		}

		sshArgs := buildSSHArgs(server, nil, host)

		type checkStep struct{ name, label, sshCmd, okKeyword string }
		preSteps := []checkStep{
			{"ssh_connect", "SSH 连接", "echo ok", "ok"},
			{"root_privilege", "Root 权限", "id -u", "0"},
			{"swap_disabled", "Swap 状态", "swapon --show 2>/dev/null | wc -l", "0"},
			{"os_compatible", "操作系统", "uname -m", ""},
			{"disk_space", "磁盘空间", "df -BG / | tail -1 | awk '{print $4}' | sed 's/G//'", ""},
		}

		for _, ps := range preSteps {
			if aborted() {
				return
			}
			idx++
			send(ps.name, ps.label, wsStatusRunning, "检测中...")
			out, err := sshExec(sshTimeout, append(sshArgs, ps.sshCmd))
			if err != nil {
				fail(ps.name, ps.label, fmt.Sprintf("失败: %v", err))
				return
			}
			result := strings.TrimSpace(string(out))
			pass := result == ps.okKeyword
			if ps.name == "os_compatible" {
				pass = strings.Contains(result, "x86_64") || strings.Contains(result, "aarch64")
			} else if ps.name == "disk_space" {
				gb := 0
				fmt.Sscanf(result, "%d", &gb)
				pass = gb >= 2
				result = fmt.Sprintf("可用 %dGB", gb)
			}
			if !pass {
				fail(ps.name, ps.label, result)
				return
			}
			send(ps.name, ps.label, wsStatusSuccess, result)
		}

		// Phase 2: Tailscale
		idx++
		send("check_tailscale", "检测 Tailscale", wsStatusRunning, "检测中...")
		_, tsLookupErr := sshExec(20*time.Second, append(sshArgs, "which tailscale"))
		if tsLookupErr != nil {
			send("check_tailscale", "安装 Tailscale", wsStatusRunning, "未安装，正在安装...")
			out, err := sshExec(120*time.Second, append(sshArgs,
				"curl -fsSL https://tailscale.com/install.sh | sh"))
			if err != nil {
				fail("check_tailscale", "安装 Tailscale", fmt.Sprintf("失败: %v — %s", err, out))
				return
			}
		}
		send("check_tailscale", "检测 Tailscale", wsStatusSuccess, "已就绪")

		idx++
		send("register_tailscale", "注册 Tailscale", wsStatusRunning, "正在注册...")
		encAuth, _ := h.store.GetSystemConfig("tailscale_auth_key")
		if encAuth == "" {
			fail("register_tailscale", "注册 Tailscale", "Auth Key 未配置")
			return
		}
		// Run tailscale up via SSH — exec args separately prevent injection
		out2, err := sshExec(60*time.Second, append(sshArgs,
			"tailscale", "up", "--reset", "--auth-key="+encAuth, "--accept-routes"))
		if err != nil {
			fail("register_tailscale", "注册 Tailscale", fmt.Sprintf("失败: %v — %s", err, out2))
			return
		}
		send("register_tailscale", "注册 Tailscale", wsStatusSuccess, "已注册")

		idx++
		send("get_tailscale_ip", "获取 Tailscale IP", wsStatusRunning, "获取中...")
		ipOut, err := sshExec(10*time.Second, append(sshArgs, "tailscale ip -4"))
		tsIP := ""
		if err == nil {
			tsIP = strings.TrimSpace(string(ipOut))
		}
		send("get_tailscale_ip", "获取 Tailscale IP", wsStatusSuccess, tsIP)

		// Phase 3: k3s agent
		encToken, _ := h.store.GetSystemConfig("k3s_join_token")
		var controlTSIP string
		servers, _ := h.store.ListServers()
		for _, srv := range servers {
			if srv.ClusterRole == "control-plane" && srv.TailscaleIP != "" {
				controlTSIP = srv.TailscaleIP
				break
			}
		}

		idx++
		send("install_k3s_agent", "安装 k3s-agent", wsStatusRunning, "正在安装...")
		if encToken == "" || controlTSIP == "" || tsIP == "" {
			fail("install_k3s_agent", "安装 k3s-agent", "Token/IP 不可用")
			return
		}
		// Build install command — use positional args via heredoc-safe approach
		installCmd := fmt.Sprintf(
			`curl -sfL https://get.k3s.io | K3S_URL='https://%s:6443' K3S_TOKEN='%s' INSTALL_K3S_EXEC='--node-ip=%s' sh -`,
			shellEscape(controlTSIP), shellEscape(encToken), shellEscape(tsIP))
		out3, err := sshExec(180*time.Second, append(sshArgs, installCmd))
		if err != nil {
			fail("install_k3s_agent", "安装 k3s-agent", fmt.Sprintf("失败: %v — %s", err, out3))
			return
		}
		send("install_k3s_agent", "安装 k3s-agent", wsStatusSuccess, "已安装")

		idx++
		send("start_service", "启动服务", wsStatusRunning, "等待 k3s-agent 启动...")
		out4, err := sshExec(30*time.Second, append(sshArgs, "systemctl is-active k3s-agent"))
		if err != nil {
			fail("start_service", "启动服务", fmt.Sprintf("失败: %s", out4))
			return
		}
		send("start_service", "启动服务", wsStatusSuccess, "已启动")

		idx++
		send("wait_ready", "等待节点就绪", wsStatusRunning, "等待注册到集群...")
		ready := false
		for i := 0; i < 60; i++ { // 300s total
			if aborted() {
				return
			}
			time.Sleep(5 * time.Second)
			if K8s != nil {
				node, ndErr := K8s.GetNodeInfo(server.K8sNodeName)
				if ndErr == nil && node != nil && node.Ready {
					ready = true
					break
				}
			}
		}
		if !ready {
			fail("wait_ready", "等待节点就绪", "超时 (300s)")
			return
		}
		send("wait_ready", "等待节点就绪", wsStatusSuccess, "节点已就绪")

		// Update DB
		server.ClusterRole = "worker"
		hostnameOut, _ := sshExec(10*time.Second, append(sshArgs, "hostname"))
		server.K8sNodeName = strings.TrimSpace(string(hostnameOut))
		server.TailscaleIP = tsIP
		server.TailscaleOnline = true
		h.store.UpdateServer(server)

		idx++
		send("complete", "加入完成", wsStatusSuccess, fmt.Sprintf("节点 %s 已加入集群", server.Name))
	}()
}

// shellEscape wraps a string in single quotes for safe shell usage.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
