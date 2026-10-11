package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type networkMigrationNode struct {
	ServerID uint   `json:"server_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	IP       string `json:"tailscale_ip,omitempty"`
	Ready    bool   `json:"ready"`
	Reason   string `json:"reason,omitempty"`
	server   model.Server
	start    string
}

type networkMigrationCheck struct {
	Ready   bool                   `json:"ready"`
	Nodes   []networkMigrationNode `json:"nodes"`
	Message string                 `json:"message,omitempty"`
}

func (h *DeploymentHandler) NetworkMigrationPrecheck(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	result, err := h.checkNetworkMigration(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取集群组网信息失败")
		return
	}
	apiShared.Success(c, result)
}

func (h *DeploymentHandler) StartNetworkMigration(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	check, err := h.checkNetworkMigration(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "迁移预检失败")
		return
	}
	if !check.Ready {
		apiShared.BadRequest(c, "迁移预检未通过，请查看组网页的节点检查结果")
		return
	}
	task, err := h.store.StartClusterNetworkMigration(id)
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "集群状态已变化或有其他任务正在执行，请重新预检")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建组网迁移任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "K3s 组网迁移任务已创建", apiShared.EnsureRequestID(c))
	go h.runNetworkMigration(task, check.Nodes)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) checkNetworkMigration(ctx context.Context, id uint) (*networkMigrationCheck, error) {
	cluster, err := h.store.GetCluster(id)
	if err != nil {
		return nil, err
	}
	result := &networkMigrationCheck{Nodes: []networkMigrationNode{}}
	if cluster.Distribution != "k3s" || cluster.ControlServerID == nil || cluster.Status != "ready" {
		result.Message = "仅支持已运行、单控制平面的 K3s 集群"
		return result, nil
	}
	if cluster.NetworkMode == "tailscale" {
		result.Message = "集群已配置为 Tailscale 组网"
		return result, nil
	}
	servers, err := h.store.ListServers()
	if err != nil {
		return nil, err
	}
	var control *model.Server
	for i := range servers {
		if servers[i].ClusterID == nil || *servers[i].ClusterID != id {
			continue
		}
		if servers[i].ClusterRole == "control-plane" && servers[i].ID == *cluster.ControlServerID {
			control = &servers[i]
		}
		result.Nodes = append(result.Nodes, networkMigrationNode{ServerID: servers[i].ID, Name: servers[i].Name, Role: servers[i].ClusterRole, server: servers[i]})
	}
	if control == nil || len(result.Nodes) == 0 {
		result.Message = "控制平面服务器未绑定，请先检查集群拓扑"
		return result, nil
	}
	var kubeNodes map[string]struct {
		IP    string
		Ready bool
	}
	output, err := transport.SSHExecServerContext(ctx, 20*time.Second, control, h.key, privilegedCommand(control, "k3s kubectl get nodes -o json"))
	if err == nil {
		kubeNodes, err = parseMigrationKubeNodes(output)
	}
	if err != nil {
		result.Message = "无法从控制平面读取 Kubernetes 节点，请先恢复集群 API"
		return result, nil
	}
	if len(kubeNodes) != len(result.Nodes) {
		result.Message = "Kubernetes 节点数与已纳管服务器数不一致，请先纳管全部节点"
		return result, nil
	}
	seen := map[string]bool{}
	for i := range result.Nodes {
		node := &result.Nodes[i]
		if node.Role != "control-plane" && node.Role != "worker" {
			node.Reason = "不支持的节点角色"
			continue
		}
		if node.Role == "control-plane" && node.ServerID != control.ID {
			node.Reason = "仅支持单控制平面迁移"
			continue
		}
		status := inspectTailscale(ctx, &node.server, h.key)
		if status.Status != "online" || net.ParseIP(status.IP) == nil {
			node.Reason = "请先到服务器组网页安装并接入 Tailscale"
			continue
		}
		node.IP = status.IP
		if seen[node.IP] {
			node.Reason = "Tailscale IP 重复"
			continue
		}
		seen[node.IP] = true
		if node.server.K8sNodeName == "" {
			node.Reason = "服务器缺少对应的 K3s 节点名"
			continue
		}
		current, ok := kubeNodes[strings.ToLower(node.server.K8sNodeName)]
		if !ok || !current.Ready {
			node.Reason = "K3s 节点不存在或未就绪"
			continue
		}
		service := migrationService(node.Role)
		command := privilegedCommand(&node.server, "systemctl is-active "+service+" && systemctl show -p FragmentPath --value "+service+" && systemctl show -p DropInPaths --value "+service+" && cat /etc/systemd/system/"+service+".service")
		out, err := transport.SSHExecServerContext(ctx, 20*time.Second, &node.server, h.key, command)
		if err != nil {
			node.Reason = "无法读取 K3s 服务配置或缺少免密 sudo 权限"
			continue
		}
		start, err := parseMigrationUnit(string(out), service, node.Role)
		if err != nil {
			node.Reason = err.Error()
			continue
		}
		node.start = start
		node.Ready = true
	}
	controlIP := ""
	for _, node := range result.Nodes {
		if node.ServerID == control.ID {
			controlIP = node.IP
			break
		}
	}
	if controlIP != "" {
		for i := range result.Nodes {
			node := &result.Nodes[i]
			if !node.Ready || node.Role != "worker" {
				continue
			}
			_, err := transport.SSHExecServerContext(ctx, 15*time.Second, &node.server, h.key, "tailscale ping --c 1 "+controlIP+" >/dev/null && curl -ksS --connect-timeout 5 --max-time 8 -o /dev/null https://"+net.JoinHostPort(controlIP, "6443")+"/")
			if err != nil {
				node.Ready = false
				node.Reason = "无法经 Tailscale 访问控制平面 K3s API（6443）"
			}
		}
	}
	result.Ready = len(result.Nodes) > 0
	for _, node := range result.Nodes {
		if !node.Ready {
			result.Ready = false
		}
	}
	if !result.Ready {
		result.Message = "请先处理未通过检查的节点，再重新预检"
	}
	return result, nil
}

func migrationService(role string) string {
	if role == "worker" {
		return "k3s-agent"
	}
	return "k3s"
}

func privilegedCommand(server *model.Server, command string) string {
	if server.SSHUser == "root" {
		return command
	}
	return "sudo -n sh -c " + shellQuote(command)
}

var migrationStartPattern = regexp.MustCompile(`^/usr/local/bin/k3s\s+(server|agent)(\s|$)`)

func parseMigrationUnit(output, service, role string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "active" || strings.TrimSpace(lines[1]) != "/etc/systemd/system/"+service+".service" || strings.TrimSpace(lines[2]) != "" {
		return "", fmt.Errorf("K3s 服务未运行或存在自定义 systemd 配置，暂不支持自动迁移")
	}
	var starts []string
	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		start := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		for strings.HasSuffix(start, "\\") && i+1 < len(lines) {
			start = strings.TrimSpace(strings.TrimSuffix(start, "\\")) + " " + strings.TrimSpace(lines[i+1])
			i++
		}
		starts = append(starts, strings.TrimSpace(start))
	}
	if len(starts) != 1 || !migrationStartPattern.MatchString(starts[0]) || strings.ContainsAny(starts[0], ";|&$%`\n") {
		return "", fmt.Errorf("K3s 启动参数无法安全识别，暂不支持自动迁移")
	}
	if role == "worker" && !strings.HasPrefix(starts[0], "/usr/local/bin/k3s agent") {
		return "", fmt.Errorf("工作节点服务类型不匹配")
	}
	if role == "control-plane" && !strings.HasPrefix(starts[0], "/usr/local/bin/k3s server") {
		return "", fmt.Errorf("控制平面服务类型不匹配")
	}
	for _, flag := range []string{"--node-ip", "--flannel-iface", "--advertise-address", "--server"} {
		if strings.Contains(starts[0], flag) {
			return "", fmt.Errorf("K3s 已有自定义网络参数，暂不支持自动迁移")
		}
	}
	return starts[0], nil
}

func parseMigrationKubeNodes(output []byte) (map[string]struct {
	IP    string
	Ready bool
}, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Addresses []struct {
					Type    string `json:"type"`
					Address string `json:"address"`
				} `json:"addresses"`
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &list); err != nil {
		return nil, err
	}
	result := make(map[string]struct {
		IP    string
		Ready bool
	})
	for _, item := range list.Items {
		state := struct {
			IP    string
			Ready bool
		}{}
		for _, address := range item.Status.Addresses {
			if address.Type == "InternalIP" {
				state.IP = address.Address
			}
		}
		for _, condition := range item.Status.Conditions {
			if condition.Type == "Ready" {
				state.Ready = condition.Status == "True"
			}
		}
		result[strings.ToLower(item.Metadata.Name)] = state
	}
	return result, nil
}

func (h *DeploymentHandler) runNetworkMigration(task *model.ClusterDeployment, nodes []networkMigrationNode) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	log("开始迁移 K3s 节点网络至 Tailscale")
	var control networkMigrationNode
	for _, node := range nodes {
		if node.Role == "control-plane" {
			control = node
			break
		}
	}
	changed := make([]networkMigrationNode, 0, len(nodes))
	ordered := []networkMigrationNode{control}
	for _, node := range nodes {
		if node.ServerID != control.ServerID {
			ordered = append(ordered, node)
		}
	}
	for _, node := range ordered {
		log("配置 " + node.Name + " 的 K3s Tailscale 地址")
		changed = append(changed, node)
		if err := h.applyNetworkMigration(ctx, node, control.IP, log); err != nil {
			log("节点配置失败：" + node.Name + "（" + err.Error() + "）；开始恢复已修改节点")
			h.rollbackNetworkMigration(changed, log)
			_ = h.store.FinishClusterNetworkMigration(task, false, "")
			_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 组网迁移失败", "")
			return
		}
		log("等待 " + node.Name + " 在 Kubernetes 中以 Tailscale IP 就绪")
		if err := h.waitMigrationNode(ctx, control.server, node, log); err != nil {
			log("节点验证失败：" + node.Name + "（" + err.Error() + "）；开始恢复已修改节点")
			h.rollbackNetworkMigration(changed, log)
			_ = h.store.FinishClusterNetworkMigration(task, false, "")
			_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 组网迁移失败", "")
			return
		}
		log(node.Name + " 已就绪，K3s 节点 IP 已切换至 " + node.IP)
	}
	if err := h.store.FinishClusterNetworkMigration(task, true, control.IP); err != nil {
		log("节点已迁移，但保存集群状态失败，请检查平台数据库")
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存 K3s 组网迁移状态失败", "")
		return
	}
	log("所有节点迁移完成，已保存控制平面的 Tailscale IP")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "K3s 组网迁移成功", "")
}

func (h *DeploymentHandler) applyNetworkMigration(ctx context.Context, node networkMigrationNode, controlIP string, log func(string)) error {
	service := migrationService(node.Role)
	start := node.start + " --node-ip " + node.IP + " --flannel-iface tailscale0"
	if node.Role == "control-plane" {
		start += " --advertise-address " + node.IP + " --tls-san " + node.IP
	} else {
		start += " --server https://" + net.JoinHostPort(controlIP, "6443")
	}
	content := "[Service]\nExecStart=\nExecStart=" + start + "\n"
	path := "/etc/systemd/system/" + service + ".service.d/90-cylism-tailscale.conf"
	command := "install -d -m 0755 /etc/systemd/system/" + service + ".service.d && " +
		"tee " + path + ".tmp >/dev/null && chmod 0644 " + path + ".tmp && mv " + path + ".tmp " + path
	if err := h.runNetworkStdin(ctx, &node.server, command, content); err != nil {
		return err
	}
	log("已写入 " + node.Name + " 的 K3s systemd 覆盖配置，重启 " + service)
	_, err := transport.SSHExecServerContext(ctx, 90*time.Second, &node.server, h.key, privilegedCommand(&node.server, "systemctl daemon-reload && systemctl restart "+service))
	return err
}

func (h *DeploymentHandler) runNetworkStdin(ctx context.Context, server *model.Server, command, input string) error {
	if server.SSHUser != "root" {
		command = "sudo -n sh -c " + shellQuote(command)
	}
	run, err := transport.StartSSHCommandContext(ctx, 90*time.Second, server, h.key, command)
	if err != nil {
		return err
	}
	defer run.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, run.Stdout) }()
	go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, run.Stderr) }()
	_, writeErr := io.WriteString(run.Stdin, input)
	_ = run.Stdin.Close()
	wg.Wait()
	if writeErr != nil {
		return writeErr
	}
	return run.Wait()
}

func (h *DeploymentHandler) rollbackNetworkMigration(nodes []networkMigrationNode, log func(string)) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for i := len(nodes) - 1; i >= 0; i-- {
		node := nodes[i]
		service := migrationService(node.Role)
		path := "/etc/systemd/system/" + service + ".service.d/90-cylism-tailscale.conf"
		command := "rm -f " + path + " && systemctl daemon-reload && systemctl restart " + service
		_, err := transport.SSHExecServerContext(ctx, 90*time.Second, &node.server, h.key, privilegedCommand(&node.server, command))
		if err != nil {
			log("恢复 " + node.Name + " 失败，请检查 systemd 配置和 K3s 服务")
		} else {
			log("已恢复 " + node.Name + " 的原有 K3s 启动配置")
		}
	}
}

func (h *DeploymentHandler) waitMigrationNode(ctx context.Context, control model.Server, node networkMigrationNode, log func(string)) error {
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	checks := 0
	for {
		output, err := transport.SSHExecServerContext(ctx, 20*time.Second, &control, h.key, privilegedCommand(&control, "k3s kubectl get nodes -o json"))
		if err == nil {
			states, parseErr := parseMigrationKubeNodes(bytes.TrimSpace(output))
			if parseErr == nil {
				state := states[strings.ToLower(node.server.K8sNodeName)]
				if state.Ready && state.IP == node.IP {
					return nil
				}
			}
		}
		checks++
		if checks%6 == 0 {
			log("仍在等待 " + node.Name + " 的 Node Ready 与 IP 更新")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("等待节点就绪超时")
		case <-time.After(5 * time.Second):
		}
	}
}
