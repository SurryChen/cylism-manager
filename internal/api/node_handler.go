package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// NodeHandler K8s 节点管理的 HTTP handler
type NodeHandler struct {
	store  *store.Store
	encKey []byte
}

type drainNodeRequest struct {
	DeleteEmptyDirData bool `json:"delete_empty_dir_data"`
}

type forceDrainNodeRequest struct {
	DeleteEmptyDirData bool   `json:"delete_empty_dir_data"`
	AcknowledgeRisk    bool   `json:"acknowledge_risk"`
	ConfirmNodeName    string `json:"confirm_node_name"`
}

// NewNodeHandler 创建 NodeHandler
func NewNodeHandler(s *store.Store, encKey []byte) *NodeHandler {
	return &NodeHandler{store: s, encKey: encKey}
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

// DrainPlan previews affected Pods before an operator cordons a node.
func (h *NodeHandler) DrainPlan(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	plan, err := K8s.DrainPlan(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, plan)
}

// DrainNode cordons the node and sends PDB-aware eviction requests.
func (h *NodeHandler) DrainNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var request drainNodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "驱逐选项无效")
		return
	}
	result, err := K8s.DrainNode(c.Param("id"), k8s.DrainOptions{DeleteEmptyDirData: request.DeleteEmptyDirData})
	if err != nil {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), result)
		return
	}
	message := "驱逐请求已提交"
	if len(result.Pending) > 0 {
		message = "部分 Pod 暂未迁移，请查看逐 Pod 原因后重试"
	}
	model.SuccessWithMessage(c, result, message)
}

// ForceDrainNode handles confirmed failure recovery. The K8s layer permits it
// only for failed worker nodes and never deletes unmanaged Pods.
func (h *NodeHandler) ForceDrainNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	var request forceDrainNodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "强制驱逐选项无效")
		return
	}
	if !request.AcknowledgeRisk || strings.TrimSpace(request.ConfirmNodeName) != c.Param("id") {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, "请确认风险并输入目标节点名")
		return
	}
	result, err := K8s.ForceDrainNode(c.Param("id"), k8s.ForceDrainOptions{
		DeleteEmptyDirData: request.DeleteEmptyDirData,
		AcknowledgeRisk:    request.AcknowledgeRisk,
		ConfirmNodeName:    strings.TrimSpace(request.ConfirmNodeName),
	})
	if err != nil {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), result)
		return
	}
	model.SuccessWithMessage(c, result, "故障节点强制驱逐请求已提交")
}

func (h *NodeHandler) RemovalCheck(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	check, err := K8s.NodeRemovalCheck(c.Param("id"))
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, check)
}

// RemoveNode 从集群移除节点
func (h *NodeHandler) RemoveNode(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if err := K8s.DeleteNode(c.Param("id")); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	if err := h.store.UnbindServersFromClusterNode(c.Param("id")); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "节点已从集群移除，但解除服务器绑定失败: "+err.Error())
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

		host := server.Host

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

		sshArgs := buildSSHArgs(server, h.encKey, host)

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

		// Phase 3: k3s agent
		encToken, _ := h.store.GetSystemConfig("k3s_join_token")
		var controlTSIP string
		servers, _ := h.store.ListServers()
		for _, srv := range servers {
			if srv.ClusterRole == "control-plane" && srv.Host != "" {
				controlTSIP = srv.Host
				break
			}
		}

		idx++
		send("install_k3s_agent", "安装 k3s-agent", wsStatusRunning, "正在安装...")
		if encToken == "" || controlTSIP == "" {
			fail("install_k3s_agent", "安装 k3s-agent", "Token/IP 不可用")
			return
		}
		// Build install command — use positional args via heredoc-safe approach
		installCmd := fmt.Sprintf(
			`curl -sfL https://get.k3s.io | K3S_URL='https://%s:6443' K3S_TOKEN='%s' sh -`,
			shellEscape(controlTSIP), shellEscape(encToken))
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
		h.store.UpdateServer(server)

		idx++
		send("complete", "加入完成", wsStatusSuccess, fmt.Sprintf("节点 %s 已加入集群", server.Name))
	}()
}

// shellEscape wraps a string in single quotes for safe shell usage.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// cleanHostname extracts the first non-empty line from SSH combined output,
// stripping stderr noise like known_hosts warnings.
func cleanHostname(raw string) string {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(trimmed, "Warning:") || strings.Contains(trimmed, "Permanently") {
			continue
		}
		return trimmed
	}
	return ""
}

// PreImport 导入预检（不写DB） POST /api/nodes/:id/preimport
func (h *NodeHandler) PreImport(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}

	host := server.Host
	sshArgs := buildSSHArgs(server, h.encKey, host)

	// 1. SSH 连接
	out, err := sshExec(sshTimeout, append(sshArgs, "echo ok"))
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeInternalError,
			fmt.Sprintf("SSH 连接失败: %v — %s", err, strings.TrimSpace(string(out))))
		return
	}

	// 2. 获取主机名（ssh combined output 可能混入 stderr 如 known_hosts warning）
	hostnameOut, err2 := sshExec(sshTimeout, append(sshArgs, "hostname"))
	if err2 != nil {
		model.Error(c, http.StatusOK, model.CodeInternalError,
			fmt.Sprintf("获取主机名失败: %v", err2))
		return
	}
	hostname := cleanHostname(string(hostnameOut))
	if hostname == "" {
		model.Error(c, http.StatusOK, model.CodeInternalError, "主机名为空")
		return
	}

	// 3. 匹配 k8s 节点（优先用 Host IP 匹配 InternalIP，fallback 用 hostname）
	if K8s == nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, "K8s 客户端未初始化")
		return
	}
	var nodeInfo *k8s.NodeInfo
	allNodes, listErr := K8s.ListNodeInfos()
	if listErr != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, "获取集群节点列表失败")
		return
	}
	// 策略1: 用服务器 Host IP 匹配 InternalIP
	for i := range allNodes {
		if allNodes[i].InternalIP == server.Host {
			nodeInfo = &allNodes[i]
			break
		}
	}
	// 策略2: fallback 用 hostname 匹配（大小写不敏感）
	if nodeInfo == nil {
		for i := range allNodes {
			if strings.EqualFold(allNodes[i].Name, hostname) {
				nodeInfo = &allNodes[i]
				break
			}
		}
	}
	if nodeInfo == nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError,
			fmt.Sprintf("集群中未找到匹配节点 (IP=%s, hostname=%s)", server.Host, hostname))
		return
	}
	if !nodeInfo.Ready {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError,
			fmt.Sprintf("节点 %s 状态异常 (NotReady)", nodeInfo.Name))
		return
	}

	model.Success(c, gin.H{
		"server_name": server.Name,
		"hostname":    hostname,
		"node_name":   nodeInfo.Name,
		"role":        nodeInfo.Roles,
		"version":     nodeInfo.Version,
		"internal_ip": nodeInfo.InternalIP,
		"os":          nodeInfo.OS,
	})
}

// ConfirmImport 确认导入 POST /api/nodes/:id/import
func (h *NodeHandler) ConfirmImport(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}

	var req struct {
		Hostname string `json:"hostname" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}

	server.K8sNodeName = req.Hostname
	server.ClusterRole = req.Role
	h.store.UpdateServer(server)

	model.SuccessWithMessage(c, gin.H{
		"server_name": server.Name,
		"node_name":   req.Hostname,
		"role":        req.Role,
	}, "导入成功")
}
