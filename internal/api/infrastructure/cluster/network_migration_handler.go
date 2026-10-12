package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type networkMigrationNode struct {
	ServerID  uint   `json:"server_id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	IP        string `json:"tailscale_ip,omitempty"`
	CurrentIP string `json:"current_ip,omitempty"`
	Ready     bool   `json:"ready"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	server    model.Server
	start     string
	backend   string
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
	if !check.Ready || !allMigrationNodesConfigured(check.Nodes) {
		apiShared.BadRequest(c, "仍有节点需要迁移或统一启动参数，请先完成逐节点操作")
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

func allMigrationNodesConfigured(nodes []networkMigrationNode) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if !node.Ready || node.Action != "skip" {
			return false
		}
	}
	return true
}

func (h *DeploymentHandler) StartNetworkNodeMigration(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	serverID, err := apiShared.ParsePositiveID(c.Param("serverID"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return
	}
	check, err := h.checkNetworkMigration(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "节点组网预检失败")
		return
	}
	var selected *networkMigrationNode
	var controlIP string
	for i := range check.Nodes {
		if check.Nodes[i].Role == "control-plane" {
			controlIP = check.Nodes[i].IP
		}
		if check.Nodes[i].ServerID == serverID {
			selected = &check.Nodes[i]
		}
	}
	if selected == nil || !selected.Ready || (selected.Action != "migrate" && selected.Action != "normalize") || net.ParseIP(controlIP) == nil || selected.start == "" {
		apiShared.BadRequest(c, "该节点当前不可操作，请重新预检并查看节点原因")
		return
	}
	operation := "network-migrate-node"
	if selected.Action == "normalize" {
		operation = "network-normalize-node"
	}
	task, err := h.store.StartClusterNetworkNodeMigration(id, serverID, operation)
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "集群状态已变化或有其他任务正在执行，请重新预检")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建节点组网任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "K3s 节点组网任务已创建", apiShared.EnsureRequestID(c))
	go h.runNetworkNodeMigration(task, *selected, check.Nodes, controlIP)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) runNetworkNodeMigration(task *model.ClusterDeployment, node networkMigrationNode, nodes []networkMigrationNode, controlIP string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	appendLog := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	appendLog("开始配置 " + node.Name + " 的 K3s Tailscale 启动参数")
	var control model.Server
	for _, candidate := range nodes {
		if candidate.Role == "control-plane" {
			control = candidate.server
			break
		}
	}
	err := h.applyNetworkMigration(ctx, node, controlIP, appendLog)
	if err == nil {
		appendLog("等待 " + node.Name + " 的 Kubernetes Node 恢复就绪")
		err = h.waitMigrationNode(ctx, control, node, appendLog)
	}
	if err == nil {
		var facts migrationConfig
		facts, err = h.readMigrationConfig(ctx, &node)
		if err == nil {
			if reason := configuredTailscaleReason(facts, node.Role, node.IP, controlIP); reason != "" {
				err = fmt.Errorf("重启后参数校验失败：%s", reason)
			}
		}
	}
	if err != nil {
		appendLog("节点组网失败：" + err.Error() + "；尝试恢复原启动配置")
		h.rollbackNetworkMigration([]networkMigrationNode{node}, appendLog)
		_ = h.store.FinishClusterNetworkNodeMigration(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 节点组网失败", "")
		return
	}
	if err := h.store.FinishClusterNetworkNodeMigration(task, true); err != nil {
		appendLog("节点已配置，但保存任务状态失败，请检查平台数据库")
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存节点组网状态失败", "")
		return
	}
	appendLog(node.Name + " 已就绪，启动参数已确认；请重新预检其他节点")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "K3s 节点组网成功", "")
}

func (h *DeploymentHandler) checkNetworkMigration(ctx context.Context, id uint) (*networkMigrationCheck, error) {
	started := time.Now()
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
		result.Nodes = append(result.Nodes, networkMigrationNode{ServerID: servers[i].ID, Name: servers[i].Name, Role: servers[i].ClusterRole, Action: "block", server: servers[i]})
	}
	if control == nil || len(result.Nodes) == 0 {
		result.Message = "控制平面服务器未绑定，请先检查集群拓扑"
		return result, nil
	}
	var kubeNodes map[string]struct {
		IP    string
		Ready bool
	}
	output, err := h.execMigrationPrecheck(ctx, 20*time.Second, control, privilegedCommand(control, "k3s kubectl get nodes -o json"))
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
	nodesDone := time.Now()
	controlStatus := h.inspectMigrationTailscale(ctx, control)
	controlIP := ""
	if controlStatus.Status == "online" && net.ParseIP(controlStatus.IP) != nil {
		controlIP = controlStatus.IP
	}
	statuses := make([]tailscaleStatus, len(result.Nodes))
	parallelMigrationChecks(len(result.Nodes), func(i int) {
		if result.Nodes[i].ServerID == control.ID {
			statuses[i] = controlStatus
			return
		}
		statuses[i] = h.inspectMigrationTailscale(ctx, &result.Nodes[i].server)
	})
	statusesDone := time.Now()
	seen := map[string]bool{}
	var candidates []int
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
		status := statuses[i]
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
		node.CurrentIP = current.IP
		candidates = append(candidates, i)
	}
	parallelMigrationChecks(len(candidates), func(i int) {
		h.checkMigrationNodeConfig(ctx, &result.Nodes[candidates[i]], controlIP)
	})
	configsDone := time.Now()
	if controlIP != "" {
		for _, node := range result.Nodes {
			if node.Action == "skip" && node.backend != "" && node.backend != "vxlan" {
				for i := range result.Nodes {
					if result.Nodes[i].Action == "migrate" {
						result.Nodes[i].Action = "block"
						result.Nodes[i].Ready = false
						result.Nodes[i].Reason = "已有节点使用不同的 Flannel 后端，需先核对跨节点 Pod 网络"
					}
				}
				break
			}
		}
	}
	if controlIP != "" {
		for i := range result.Nodes {
			node := &result.Nodes[i]
			if !node.Ready || node.Role != "worker" || node.Action != "migrate" {
				continue
			}
			_, err := h.execMigrationPrecheck(ctx, 15*time.Second, &node.server, "tailscale ping --c 1 "+controlIP+" >/dev/null && curl -ksS --connect-timeout 5 --max-time 8 -o /dev/null https://"+net.JoinHostPort(controlIP, "6443")+"/")
			if err != nil {
				node.Ready = false
				node.Action = "block"
				node.Reason = "无法经 Tailscale 访问控制平面 K3s API（6443）"
			}
		}
	}
	result.Ready = len(result.Nodes) > 0
	for _, node := range result.Nodes {
		log.Printf("[cluster-network-migration] precheck cluster=%d server=%d node=%q k3s_ip=%q tailscale_ip=%q action=%q ready=%t reason=%q", id, node.ServerID, node.server.K8sNodeName, node.CurrentIP, node.IP, node.Action, node.Ready, node.Reason)
		if !node.Ready {
			result.Ready = false
		}
	}
	if !result.Ready {
		result.Message = "请先处理未通过检查的节点，再重新预检"
	}
	finished := time.Now()
	log.Printf("[cluster-network-migration] precheck timing cluster=%d nodes=%s tailscale=%s config=%s connectivity=%s total=%s", id, nodesDone.Sub(started), statusesDone.Sub(nodesDone), configsDone.Sub(statusesDone), finished.Sub(configsDone), finished.Sub(started))
	return result, nil
}

func parallelMigrationChecks(count int, check func(int)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := 0; i < count; i++ {
		slots <- struct{}{}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() { <-slots }()
			check(index)
		}(i)
	}
	wg.Wait()
}

func (h *DeploymentHandler) inspectMigrationTailscale(ctx context.Context, server *model.Server) tailscaleStatus {
	checkCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return inspectTailscale(checkCtx, server, h.key)
}

func (h *DeploymentHandler) execMigrationPrecheck(ctx context.Context, timeout time.Duration, server *model.Server, command string) ([]byte, error) {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return transport.SSHExecServerContext(checkCtx, timeout, server, h.key, command)
}

func (h *DeploymentHandler) checkMigrationNodeConfig(ctx context.Context, node *networkMigrationNode, controlIP string) {
	facts, err := h.readMigrationConfig(ctx, node)
	if err != nil {
		node.Reason = "无法核实 K3s 生效配置，请检查 SSH 与免密 sudo 权限"
		return
	}
	log.Printf("[cluster-network-migration] config server=%d node=%q expected_ip=%q node_ip=%q flannel_iface=%q raw_mentions_node_ip=%t raw_mentions_flannel_iface=%t unknown_reason=%q", node.ServerID, node.server.K8sNodeName, node.IP, facts.values["node-ip"], facts.values["flannel-iface"], facts.rawMentionsNodeIP, facts.rawMentionsFlannelIface, facts.unknownReason)
	node.backend = facts.values["flannel-backend"]
	if node.CurrentIP == node.IP {
		if reason := configuredTailscaleReason(facts, node.Role, node.IP, controlIP); reason == "" {
			node.Action = "skip"
			node.Ready = true
			node.Reason = "K3s 已使用 Tailscale 地址及接口，无需重启"
			return
		} else if facts.unknownReason != "" || facts.hasNetworkSettings() {
			node.Reason = reason
			return
		}
	}
	if facts.unknownReason != "" {
		node.Reason = facts.unknownReason
		return
	}
	if facts.values["server"] != "" && (node.Role != "worker" || facts.values["server"] != "https://"+net.JoinHostPort(controlIP, "6443")) {
		node.Reason = "K3s 控制平面连接地址与目标 Tailscale 地址不一致"
		return
	}
	if node.CurrentIP == "" || facts.hasNetworkSettings() {
		node.Reason = "K3s 已有其他网络配置或节点 IP 不明，请核对后再迁移"
		return
	}
	service := migrationService(node.Role)
	command := privilegedCommand(&node.server, "systemctl is-active "+service+" && systemctl show -p FragmentPath --value "+service+" && systemctl show -p DropInPaths --value "+service+" && cat /etc/systemd/system/"+service+".service")
	out, err := h.execMigrationPrecheck(ctx, 20*time.Second, &node.server, command)
	if err != nil {
		node.Reason = "无法读取 K3s 服务配置或缺少免密 sudo 权限"
		return
	}
	start, err := parseMigrationUnit(string(out), service, node.Role)
	if err != nil {
		node.Reason = err.Error()
		return
	}
	node.start = start
	node.Action = "migrate"
	if node.CurrentIP == node.IP {
		node.Action = "normalize"
		node.Reason = "节点 IP 已使用 Tailscale，需要统一 K3s 启动参数"
	}
	node.Ready = true
}

type migrationConfig struct {
	values                  map[string]string
	unknownReason           string
	rawMentionsNodeIP       bool
	rawMentionsFlannelIface bool
}

func (c migrationConfig) hasNetworkSettings() bool {
	for _, key := range []string{"node-ip", "flannel-iface", "advertise-address", "flannel-backend"} {
		if c.values[key] != "" {
			return true
		}
	}
	return false
}

func configuredTailscaleReason(config migrationConfig, role, ip, controlIP string) string {
	if config.unknownReason != "" {
		return config.unknownReason
	}
	if config.values["node-ip"] != ip || config.values["flannel-iface"] != "tailscale0" {
		return "节点 IP 与 Tailscale 一致，但无法确认 K3s 已完整使用 Tailscale 配置"
	}
	if role == "control-plane" && config.values["advertise-address"] != ip {
		return "控制平面缺少对应的 Tailscale advertise-address"
	}
	if role == "worker" && (controlIP == "" || config.values["server"] != "https://"+net.JoinHostPort(controlIP, "6443")) {
		return "工作节点连接地址不是控制平面的 Tailscale 地址"
	}
	return ""
}

func (h *DeploymentHandler) readMigrationConfig(ctx context.Context, node *networkMigrationNode) (migrationConfig, error) {
	service := migrationService(node.Role)
	// Only these known K3s files are read; unknown EnvironmentFile paths block migration.
	command := "systemctl is-active " + service + " && systemctl cat " + service + " && printf '\n---CYLISM_ENV---\n' && " +
		"for f in /etc/systemd/system/" + service + ".service.env /etc/default/" + service + " /etc/sysconfig/" + service + "; do if [ -f \"$f\" ]; then cat \"$f\"; printf '\n'; fi; done && " +
		"printf '\n---CYLISM_YAML---\n' && for f in /etc/rancher/k3s/config.yaml /etc/rancher/k3s/config.yaml.d/*.yaml; do if [ -f \"$f\" ]; then cat \"$f\"; printf '\n---\n'; fi; done"
	out, err := h.execMigrationPrecheck(ctx, 20*time.Second, &node.server, privilegedCommand(&node.server, command))
	if err != nil {
		return migrationConfig{}, err
	}
	return parseMigrationConfig(string(out))
}

var migrationFlagPattern = regexp.MustCompile(`(?:^|\s)['"]?--(node-ip|flannel-iface|advertise-address|server|flannel-backend)['"]?(?:=|\s+)['"]?([^\s'"\\]+)`)

func parseMigrationConfig(output string) (migrationConfig, error) {
	parts := strings.Split(output, "---CYLISM_ENV---")
	if len(parts) != 2 {
		return migrationConfig{}, fmt.Errorf("missing environment section")
	}
	remainder := strings.Split(parts[1], "---CYLISM_YAML---")
	if len(remainder) != 2 {
		return migrationConfig{}, fmt.Errorf("missing configuration section")
	}
	config := migrationConfig{values: map[string]string{}}
	normalized := strings.ToLower(strings.ReplaceAll(output, "_", "-"))
	config.rawMentionsNodeIP = strings.Contains(normalized, "node-ip")
	config.rawMentionsFlannelIface = strings.Contains(normalized, "flannel-iface")
	for _, document := range strings.Split(remainder[1], "\n---\n") {
		if strings.TrimSpace(document) == "" {
			continue
		}
		var values map[string]any
		if err := yaml.Unmarshal([]byte(document), &values); err != nil {
			config.unknownReason = "K3s YAML 配置无法解析，暂不支持自动迁移"
			continue
		}
		for _, key := range []string{"node-ip", "flannel-iface", "advertise-address", "server", "flannel-backend"} {
			if value, ok := values[key]; ok {
				config.values[key] = fmt.Sprint(value)
			}
		}
	}
	for _, line := range strings.Split(remainder[0], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "K3S_") || strings.HasPrefix(line, "'K3S_") || strings.HasPrefix(line, `"K3S_`) {
			entry := strings.Trim(line, "'\"")
			key, value, ok := strings.Cut(entry, "=")
			if !ok {
				config.unknownReason = "K3s 环境文件格式无法识别，暂不支持自动迁移"
				continue
			}
			key = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(key, "K3S_"), "_", "-"))
			if key == "url" {
				key = "server"
			}
			if _, exists := config.values[key]; exists || key == "node-ip" || key == "flannel-iface" || key == "advertise-address" || key == "server" || key == "flannel-backend" {
				config.values[key] = strings.Trim(value, "'\"")
			}
		}
	}
	unit := strings.ReplaceAll(parts[0], "\\\n", " ")
	serviceLine := ""
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExecStart=") {
			serviceLine = strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		}
		if strings.HasPrefix(line, "Environment=") && strings.Contains(line, "K3S_") {
			config.unknownReason = "K3s 使用了 systemd 内联环境变量，暂不支持自动迁移"
		}
		if strings.HasPrefix(line, "EnvironmentFile=") {
			path := strings.TrimPrefix(strings.TrimPrefix(line, "EnvironmentFile="), "-")
			if !knownK3sEnvironmentFile(path) {
				config.unknownReason = "K3s 使用了自定义环境文件，暂不支持自动迁移"
			}
		}
	}
	if !migrationStartPattern.MatchString(serviceLine) {
		config.unknownReason = "K3s 启动命令无法识别，暂不支持自动迁移"
	}
	for _, match := range migrationFlagPattern.FindAllStringSubmatch(serviceLine, -1) {
		config.values[match[1]] = match[2]
	}
	return config, nil
}

func knownK3sEnvironmentFile(path string) bool {
	switch path {
	case "/etc/default/%N", "/etc/sysconfig/%N", "/etc/systemd/system/k3s.service.env", "/etc/systemd/system/k3s-agent.service.env", "/etc/default/k3s", "/etc/default/k3s-agent", "/etc/sysconfig/k3s", "/etc/sysconfig/k3s-agent":
		return true
	default:
		return false
	}
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
			if address.Type == "InternalIP" && state.IP == "" {
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
	appendLog := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	appendLog("全部 K3s 节点已通过 Tailscale 配置检查，开始同步集群组网状态")
	controlIP := ""
	for _, node := range nodes {
		if !node.Ready || node.Action != "skip" {
			appendLog("节点配置状态已变化，请重新预检")
			_ = h.store.FinishClusterNetworkMigration(task, false, "")
			_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 组网状态同步失败", "")
			return
		}
		if node.Role == "control-plane" {
			controlIP = node.IP
		}
	}
	if net.ParseIP(controlIP) == nil {
		appendLog("控制平面的 Tailscale IP 不可用")
		_ = h.store.FinishClusterNetworkMigration(task, false, "")
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 组网状态同步失败", "")
		return
	}
	if err := h.store.FinishClusterNetworkMigration(task, true, controlIP); err != nil {
		appendLog("保存集群组网状态失败，请检查平台数据库")
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存 K3s 组网状态失败", "")
		return
	}
	appendLog("集群组网状态已同步，控制平面 Tailscale IP：" + controlIP)
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "K3s 组网状态同步成功", "")
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
