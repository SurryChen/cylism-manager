package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

// NodeJoinProgressHandler streams the existing worker installation workflow.
// It is kept as a transport adapter; cluster lifecycle decisions stay in the
// cluster service and the SSH operations use the shared infrastructure client.
type NodeJoinProgressHandler struct {
	store    repository.NodeJoinRepository
	encKey   []byte
	k8s      NodeJoinAdapter
	platform k3sPlatformReader
}

// NodeJoinAdapter is the single Kubernetes read required by the websocket
// join workflow. Keeping it as a port avoids coupling the handler to the full
// client surface.
type NodeJoinAdapter interface {
	GetNodeInfoContext(context.Context, string) (*k8s.NodeInfo, error)
}

type k3sPlatformReader interface {
	ServerVersionContext(context.Context) (string, error)
}

// NewNodeJoinAdapter narrows the Kubernetes dependency required by the join
// websocket before it is injected into the handler.
func NewNodeJoinAdapter(client NodeJoinAdapter) NodeJoinAdapter { return client }

const nodeJoinSSHTimeout = 15 * time.Second

func NewNodeJoinProgressHandler(st repository.NodeJoinRepository, encKey []byte, client NodeJoinAdapter) *NodeJoinProgressHandler {
	return &NodeJoinProgressHandler{store: st, encKey: append([]byte(nil), encKey...), k8s: client}
}

func (h *NodeJoinProgressHandler) WithPlatformReader(reader k3sPlatformReader) *NodeJoinProgressHandler {
	h.platform = reader
	return h
}

func (h *NodeJoinProgressHandler) JoinProgress(c *gin.Context) {
	if h.platform == nil {
		apiShared.Error(c, 503, apiShared.CodeK8sUnavailable, "无法确认当前集群是否为 K3s")
		return
	}
	version, platformErr := h.platform.ServerVersionContext(c.Request.Context())
	if platformErr != nil || !strings.Contains(strings.ToLower(version), "k3s") {
		apiShared.Error(c, 409, apiShared.CodeConflict, "当前集群不是已识别的 K3s，不能执行节点加入")
		return
	}
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		apiShared.Error(c, 404, apiShared.CodeNotFound, "server not found")
		return
	}
	conn, err := transport.WSUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	clientClosed := make(chan struct{})
	go func() { _ = conn.ReadClose(); close(clientClosed) }()
	go func() {
		defer conn.Close()
		ctx := c.Request.Context()
		total, index := 9, 0
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
			return transport.SendProgress(conn, total, index, step, label, status, detail, now())
		}
		fail := func(step, label, detail string) {
			index++
			_ = send(step, label, transport.WSStatusFailed, detail)
		}
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
			if !send(check.name, check.label, transport.WSStatusRunning, "检测中...") {
				return
			}
			out, runErr := transport.SSHExecServerContext(ctx, nodeJoinSSHTimeout, server, h.encKey, check.command)
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
			if !send(check.name, check.label, transport.WSStatusSuccess, result) {
				return
			}
		}

		index++
		if !send("install_k3s_agent", "安装 k3s-agent", transport.WSStatusRunning, "正在安装...") {
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
		install := k3sAgentInstallCommand(controlIP, token)
		if out, installErr := transport.SSHExecServerContext(ctx, 180*time.Second, server, h.encKey, install); installErr != nil {
			fail("install_k3s_agent", "安装 k3s-agent", fmt.Sprintf("失败: %v — %s", installErr, out))
			return
		}
		if !send("install_k3s_agent", "安装 k3s-agent", transport.WSStatusSuccess, "已安装") {
			return
		}

		index++
		if !send("start_service", "启动服务", transport.WSStatusRunning, "等待 k3s-agent 启动...") {
			return
		}
		if out, serviceErr := transport.SSHExecServerContext(ctx, 30*time.Second, server, h.encKey, "systemctl is-active k3s-agent"); serviceErr != nil {
			fail("start_service", "启动服务", fmt.Sprintf("失败: %s", out))
			return
		}
		if !send("start_service", "启动服务", transport.WSStatusSuccess, "已启动") {
			return
		}
		index++
		if !send("wait_ready", "等待节点就绪", transport.WSStatusRunning, "等待注册到集群...") {
			return
		}
		ready := false
		for i := 0; i < 60 && !aborted(); i++ {
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if h.k8s != nil {
				node, getErr := h.k8s.GetNodeInfoContext(ctx, server.K8sNodeName)
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
		if !send("wait_ready", "等待节点就绪", transport.WSStatusSuccess, "节点已就绪") {
			return
		}
		server.ClusterRole = "worker"
		if hostname, hostErr := transport.SSHExecServerContext(ctx, 10*time.Second, server, h.encKey, "hostname"); hostErr == nil {
			server.K8sNodeName = strings.TrimSpace(string(hostname))
		}
		_ = h.store.UpdateServer(server)
		index++
		_ = send("complete", "加入完成", transport.WSStatusSuccess, fmt.Sprintf("节点 %s 已加入集群", server.Name))
	}()
}

func shellEscape(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func k3sAgentInstallCommand(controlAddress, token string) string {
	return fmt.Sprintf(`curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -`, shellEscape("https://"+controlAddress+":6443"), shellEscape(token))
}
