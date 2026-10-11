package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tailscaleStatus struct {
	ServerID   uint   `json:"server_id"`
	Status     string `json:"status"`
	IP         string `json:"ip,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	ErrorType  string `json:"error_type,omitempty"`
	Error      string `json:"error,omitempty"`
}

type tailscaleConnection struct {
	FromID uint   `json:"from_id"`
	ToID   uint   `json:"to_id"`
	OK     bool   `json:"ok"`
	Path   string `json:"path"`
	Relay  string `json:"relay,omitempty"`
	Error  string `json:"error,omitempty"`
}

type tailscaleNetworkCheck struct {
	ServerID              uint   `json:"server_id"`
	UDP                   *bool  `json:"udp,omitempty"`
	MappingVariesByDestIP *bool  `json:"mapping_varies_by_dest_ip,omitempty"`
	Error                 string `json:"error,omitempty"`
}

type tailscaleDirectDiagnosis struct {
	Connection tailscaleConnection     `json:"connection"`
	Nodes      []tailscaleNetworkCheck `json:"nodes"`
	Findings   []string                `json:"findings"`
}

func inspectTailscale(ctx context.Context, server *model.Server, key []byte) tailscaleStatus {
	status := tailscaleStatus{ServerID: server.ID, Status: "unreachable"}
	command := "if ! command -v tailscale >/dev/null 2>&1; then printf '__TAILSCALE_NOT_INSTALLED__'; else tailscale status --json; fi"
	output, err := transport.SSHExecServerContext(ctx, 12*time.Second, server, key, command)
	if err != nil {
		status.ErrorType, status.Error = tailscaleQueryError(err, output)
		return status
	}
	if strings.TrimSpace(string(output)) == "__TAILSCALE_NOT_INSTALLED__" {
		status.Status = "not_installed"
		return status
	}
	if len(output) > 2*1024*1024 {
		status.ErrorType = "invalid_response"
		status.Error = "组网状态数据过大"
		return status
	}
	return parseTailscaleStatus(server.ID, output)
}

func tailscaleQueryError(err error, output []byte) (string, string) {
	var connectionError *transport.SSHConnectionError
	if errors.As(err, &connectionError) {
		if strings.Contains(strings.ToLower(err.Error()), "unable to authenticate") {
			return "ssh_auth", "SSH 认证失败，请检查用户名、密码或密钥"
		}
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
			return "ssh_timeout", "SSH 连接超时，请检查服务器地址、端口和防火墙"
		}
		return "ssh_connection", "SSH 连接失败，请检查服务器地址、端口和认证配置"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "command_timeout", "SSH 已连接，但读取 Tailscale 状态超时"
	}
	message := strings.ToLower(string(output))
	if strings.Contains(message, "tailscaled.sock") || strings.Contains(message, "failed to connect to local tailscaled") {
		return "tailscale_daemon", "SSH 已连接，Tailscale 已安装，但 tailscaled 服务未运行或不可用"
	}
	return "tailscale_command", "SSH 已连接，但 tailscale status 执行失败；请在服务器上运行 tailscale status --json 排查"
}

func parseTailscaleStatus(serverID uint, output []byte) tailscaleStatus {
	status := tailscaleStatus{ServerID: serverID, Status: "unreachable"}
	var result struct {
		BackendState string `json:"BackendState"`
		Self         struct {
			DNSName      string   `json:"DNSName"`
			HostName     string   `json:"HostName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			Online       bool     `json:"Online"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		status.ErrorType = "invalid_response"
		status.Error = "无法解析组网状态"
		return status
	}
	status.Status = "offline"
	status.DeviceName = strings.TrimSuffix(result.Self.DNSName, ".")
	if status.DeviceName == "" {
		status.DeviceName = result.Self.HostName
	}
	for _, address := range result.Self.TailscaleIPs {
		if ip := net.ParseIP(address); ip != nil && ip.To4() != nil {
			status.IP = address
			break
		}
	}
	if result.BackendState == "Running" && result.Self.Online && status.IP != "" {
		status.Status = "online"
	}
	return status
}

func (h *ServerHandler) TailscaleStatuses(c *gin.Context) {
	servers, err := h.cluster.ListServersContext(c.Request.Context())
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	items := make([]tailscaleStatus, len(servers))
	limit := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for index := range servers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			items[index] = inspectTailscale(c.Request.Context(), &servers[index], h.encKey)
		}(index)
	}
	wg.Wait()
	apiShared.Success(c, items)
}

func (h *ServerHandler) TailscaleStatus(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	apiShared.Success(c, inspectTailscale(c.Request.Context(), server, h.encKey))
}

func (h *ServerHandler) TailscaleInstall(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	output, err := transport.SSHExecServerContext(ctx, 5*time.Minute, server, h.encKey, tailscaleInstallCommand(server.SSHUser))
	if err != nil {
		apiShared.BadRequest(c, "安装或启动 Tailscale 失败："+trimCommandOutput(output))
		return
	}
	status := inspectTailscale(ctx, server, h.encKey)
	if status.Status == "not_installed" {
		apiShared.BadRequest(c, "安装 Tailscale 后未检测到 tailscale 命令")
		return
	}
	if status.Status == "unreachable" {
		apiShared.BadRequest(c, "安装或启动 Tailscale 后检测失败："+status.Error)
		return
	}
	apiShared.Success(c, status)
}

func tailscaleInstallCommand(sshUser string) string {
	privilege := ""
	if sshUser != "root" {
		privilege = "sudo -n "
	}
	return "bash -o pipefail -c '" +
		"if ! command -v tailscale >/dev/null 2>&1; then curl -fsSL https://tailscale.com/install.sh | " + privilege + "sh; fi; " +
		"if command -v systemctl >/dev/null 2>&1; then " + privilege + "systemctl enable --now tailscaled; " +
		"else " + privilege + "service tailscaled start; fi'"
}

func (h *ServerHandler) TailscaleJoin(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	var request struct {
		AuthKey string `json:"auth_key" binding:"required"`
	}
	if c.ShouldBindJSON(&request) != nil || !strings.HasPrefix(request.AuthKey, "tskey-auth-") || strings.ContainsAny(request.AuthKey, " \t\r\n") {
		apiShared.BadRequest(c, "请输入有效的 Tailscale 授权密钥")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	command := "IFS= read -r auth_key; tailscale up --auth-key=\"$auth_key\" --accept-routes=false --accept-dns=false"
	if server.SSHUser != "root" {
		command = "IFS= read -r auth_key; sudo -n tailscale up --auth-key=\"$auth_key\" --accept-routes=false --accept-dns=false"
	}
	run, err := transport.StartSSHCommandContext(ctx, 90*time.Second, server, h.encKey, command)
	if err != nil {
		apiShared.BadRequest(c, "无法启动 Tailscale 入网操作")
		return
	}
	defer run.Close()
	if _, err := io.WriteString(run.Stdin, request.AuthKey+"\n"); err != nil {
		apiShared.BadRequest(c, "发送授权密钥失败")
		return
	}
	_ = run.Stdin.Close()
	var reads sync.WaitGroup
	reads.Add(2)
	go func() { defer reads.Done(); _, _ = io.Copy(io.Discard, run.Stdout) }()
	go func() { defer reads.Done(); _, _ = io.Copy(io.Discard, run.Stderr) }()
	reads.Wait()
	if err := run.Wait(); err != nil {
		apiShared.BadRequest(c, "加入 Tailscale 失败，请检查授权密钥、sudo 权限和远端服务状态")
		return
	}
	apiShared.Success(c, inspectTailscale(ctx, server, h.encKey))
}

func (h *ServerHandler) TailscaleLeave(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	if server.ClusterID != nil || server.ClusterRole != "" {
		apiShared.Conflict(c, "服务器仍属于集群，请先卸载集群节点")
		return
	}
	command := "tailscale logout"
	if server.SSHUser != "root" {
		command = "sudo -n tailscale logout"
	}
	if output, err := transport.SSHExecServerContext(c.Request.Context(), 30*time.Second, server, h.encKey, command); err != nil {
		apiShared.BadRequest(c, "退出 Tailscale 失败："+trimCommandOutput(output))
		return
	}
	apiShared.Success(c, inspectTailscale(c.Request.Context(), server, h.encKey))
}

func (h *ServerHandler) TailscaleConnections(c *gin.Context) {
	servers, err := h.cluster.ListServersContext(c.Request.Context())
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 75*time.Second)
	defer cancel()
	type peer struct {
		server *model.Server
		ip     string
	}
	statuses := make([]tailscaleStatus, len(servers))
	statusLimit := make(chan struct{}, 8)
	var statusWG sync.WaitGroup
	for index := range servers {
		statusWG.Add(1)
		go func(index int) {
			defer statusWG.Done()
			select {
			case statusLimit <- struct{}{}:
				defer func() { <-statusLimit }()
				statuses[index] = inspectTailscale(ctx, &servers[index], h.encKey)
			case <-ctx.Done():
			}
		}(index)
	}
	statusWG.Wait()
	peers := make([]peer, 0, len(servers))
	for index, status := range statuses {
		if status.Status == "online" {
			peers = append(peers, peer{server: &servers[index], ip: status.IP})
		}
	}
	if len(peers) > 12 {
		apiShared.BadRequest(c, "一次最多检测 12 台在线服务器")
		return
	}
	results := make([]tailscaleConnection, 0, len(peers)*(len(peers)-1))
	var checks sync.WaitGroup
	checkLimit := make(chan struct{}, 12)
	pingCount := 1
	if len(peers) <= 4 {
		pingCount = 2
	}
	for source := range peers {
		indices := make([]int, len(peers))
		for target := range peers {
			if source != target {
				indices[target] = len(results)
				results = append(results, tailscaleConnection{FromID: peers[source].server.ID, ToID: peers[target].server.ID, Path: "inconclusive"})
			}
		}
		checks.Add(1)
		go func(source int, indices []int) {
			defer checks.Done()
			select {
			case checkLimit <- struct{}{}:
				defer func() { <-checkLimit }()
			case <-ctx.Done():
				return
			}
			for target := range peers {
				if source == target {
					continue
				}
				if ctx.Err() != nil {
					results[indices[target]].Error = "检测超时，请重新检测"
					continue
				}
				results[indices[target]] = probeTailscaleConnection(ctx, peers[source].server, peers[target].ip, peers[source].server.ID, peers[target].server.ID, h.encKey, pingCount)
			}
		}(source, indices)
	}
	checks.Wait()
	apiShared.Success(c, results)
}

func (h *ServerHandler) TailscaleDiagnoseConnection(c *gin.Context) {
	var request struct {
		FromID uint `json:"from_id"`
		ToID   uint `json:"to_id"`
	}
	if c.ShouldBindJSON(&request) != nil || request.FromID == 0 || request.ToID == 0 || request.FromID == request.ToID {
		apiShared.BadRequest(c, "请选择两个不同的服务器进行直连分析")
		return
	}
	from, err := h.cluster.GetServer(request.FromID)
	if err != nil {
		apiShared.NotFound(c, "来源服务器不存在")
		return
	}
	to, err := h.cluster.GetServer(request.ToID)
	if err != nil {
		apiShared.NotFound(c, "目标服务器不存在")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	toStatus := inspectTailscale(ctx, to, h.encKey)
	if toStatus.Status != "online" {
		apiShared.BadRequest(c, "目标服务器未连接 Tailscale，无法分析直连")
		return
	}
	result := tailscaleDirectDiagnosis{Nodes: make([]tailscaleNetworkCheck, 2)}
	var checks sync.WaitGroup
	checks.Add(3)
	go func() {
		defer checks.Done()
		result.Connection = probeTailscaleConnection(ctx, from, toStatus.IP, from.ID, to.ID, h.encKey, 3)
	}()
	for index, server := range []*model.Server{from, to} {
		go func(index int, server *model.Server) {
			defer checks.Done()
			result.Nodes[index] = inspectTailscaleNetwork(ctx, server, h.encKey)
		}(index, server)
	}
	checks.Wait()
	fromName, toName := from.Name, to.Name
	if fromName == "" {
		fromName = from.Host
	}
	if toName == "" {
		toName = to.Host
	}
	result.Findings = tailscaleDirectFindings(result.Connection, result.Nodes, fromName, toName)
	apiShared.Success(c, result)
}

func probeTailscaleConnection(ctx context.Context, server *model.Server, targetIP string, fromID, toID uint, key []byte, count int) tailscaleConnection {
	result := tailscaleConnection{FromID: fromID, ToID: toID, Path: "inconclusive"}
	timeout := 10 * time.Second
	if count > 1 {
		timeout = time.Duration(count*5+7) * time.Second
	}
	output, err := transport.SSHExecServerContext(ctx, timeout, server, key, "tailscale ping --c="+strconv.Itoa(count)+" --timeout=5s "+targetIP)
	result.OK, result.Path, result.Relay = parseTailscalePing(output)
	if !result.OK {
		if ctx.Err() != nil {
			result.Error = "检测超时，请重新检测"
		} else if err != nil && len(output) == 0 {
			var connectionError *transport.SSHConnectionError
			if errors.As(err, &connectionError) {
				_, result.Error = tailscaleQueryError(err, output)
				result.Path = "error"
			} else {
				result.Error = "节点探测命令执行失败"
				result.Path = "error"
			}
		} else {
			result.Error = "本次探测未收到回应，可重新检测或分析连接"
		}
	}
	return result
}

func parseTailscalePing(output []byte) (bool, string, string) {
	relay := ""
	for _, raw := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(strings.ToLower(line), "pong from ") {
			continue
		}
		_, via, found := strings.Cut(line, " via ")
		if !found {
			continue
		}
		endpoint := strings.Fields(via)
		if len(endpoint) == 0 {
			continue
		}
		if strings.HasPrefix(endpoint[0], "DERP(") && strings.HasSuffix(endpoint[0], ")") {
			relay = strings.TrimSuffix(strings.TrimPrefix(endpoint[0], "DERP("), ")")
			continue
		}
		return true, "direct", ""
	}
	if relay != "" {
		return true, "relay", relay
	}
	return false, "inconclusive", ""
}

func inspectTailscaleNetwork(ctx context.Context, server *model.Server, key []byte) tailscaleNetworkCheck {
	check := tailscaleNetworkCheck{ServerID: server.ID}
	output, err := transport.SSHExecServerContext(ctx, 20*time.Second, server, key, "tailscale netcheck")
	if err != nil {
		check.Error = "网络检测失败：" + trimCommandOutput(output)
		return check
	}
	for _, raw := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "*"))
		if value, found := strings.CutPrefix(line, "UDP:"); found {
			check.UDP = parseTailscaleBool(value)
		}
		if value, found := strings.CutPrefix(line, "MappingVariesByDestIP:"); found {
			check.MappingVariesByDestIP = parseTailscaleBool(value)
		}
	}
	if check.UDP == nil && check.MappingVariesByDestIP == nil {
		check.Error = "无法识别 Tailscale 网络检测结果"
	}
	return check
}

func parseTailscaleBool(value string) *bool {
	fields := strings.Fields(value)
	if len(fields) == 0 || fields[0] != "true" && fields[0] != "false" {
		return nil
	}
	result := fields[0] == "true"
	return &result
}

func tailscaleDirectFindings(connection tailscaleConnection, nodes []tailscaleNetworkCheck, fromName, toName string) []string {
	if connection.Path == "direct" {
		return []string{"重复探测后已建立直连；首次经 DERP 中继可能只是直连建立需要时间。"}
	}
	findings := make([]string, 0, 4)
	for index, node := range nodes {
		name := fromName
		if index == 1 {
			name = toName
		}
		if node.Error != "" {
			findings = append(findings, name+" 的网络检测未完成，暂时无法判断其 UDP 和 NAT 状态。")
		} else if node.UDP != nil && !*node.UDP {
			findings = append(findings, name+" 无法使用出站 UDP，请检查云安全组和系统防火墙的出站规则。")
		} else if node.MappingVariesByDestIP != nil && *node.MappingVariesByDestIP {
			findings = append(findings, name+" 的 NAT 映射随目标变化，直连可能受限。")
		}
	}
	if connection.Path == "relay" {
		findings = append(findings, "两端可通过 DERP 中继通信；若持续无法直连，请检查双方 UDP 入站（常用端口 41641）和出站规则。")
	} else if !connection.OK {
		findings = append(findings, "本次未收到节点回应，不能据此断定网络不通；请重试并确认两端 Tailscale 在线、Tailnet 访问策略及防火墙规则。")
	}
	return findings
}

func (h *ServerHandler) tailscaleServer(c *gin.Context) (*model.Server, bool) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return nil, false
	}
	server, err := h.cluster.GetServer(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return nil, false
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return nil, false
	}
	return server, true
}

func trimCommandOutput(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 300 {
		message = message[:300]
	}
	if message == "" {
		return "远端命令执行失败"
	}
	return message
}
