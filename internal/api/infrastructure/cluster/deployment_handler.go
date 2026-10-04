package cluster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type clusterDeploymentStore interface {
	GetCluster(uint) (*model.Cluster, error)
	GetServer(uint) (*model.Server, error)
	StartClusterDeployment(uint, uint, string, string, string) (*model.ClusterDeployment, error)
	StartClusterAdoption(uint, uint, string, string) (*model.ClusterDeployment, error)
	StartClusterUninstall(uint, uint) (*model.ClusterDeployment, error)
	FinishClusterUninstall(*model.ClusterDeployment, bool) error
	GetClusterDeployment(uint, uint) (*model.ClusterDeployment, error)
	ListClusterDeployments(uint) ([]model.ClusterDeployment, error)
	ListClusterDeploymentLogs(uint, uint) ([]model.ClusterDeploymentLog, error)
	AppendClusterDeploymentLog(uint, string) error
	FinishClusterDeployment(*model.ClusterDeployment, bool, string, string, string) error
	StartClusterAgentOperation(uint, uint, string) (*model.ClusterDeployment, error)
	FinishClusterAgentOperation(*model.ClusterDeployment, bool) error
	CreateAuditLog(*model.AuditLog) error
}

type agentCertificateIssuer interface {
	Issue(uint, string, string) (*model.AccessCertificate, error)
	GetCA() (*model.AccessCA, error)
	MarkInUse(uint, bool) error
	MarkClusterUnused(uint, string) error
}

func (h *DeploymentHandler) Uninstall(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	serverID, err := apiShared.ParsePositiveID(c.Param("serverID"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return
	}
	task, err := h.store.StartClusterUninstall(clusterID, serverID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "请先卸载工作节点，或等待当前任务结束；确认服务器属于该集群")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建卸载任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "节点卸载任务已创建", apiShared.EnsureRequestID(c))
	go h.runUninstall(task)
	apiShared.Success(c, task)
}

// AgentOperation starts an agent install or maintenance task. Every result is
// persisted in the same task and log stream used by cluster deployments.
func (h *DeploymentHandler) AgentOperation(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	var request struct {
		ServerID  uint   `json:"server_id"`
		Operation string `json:"operation"`
	}
	if c.ShouldBindJSON(&request) != nil || request.ServerID == 0 {
		apiShared.BadRequest(c, "请选择控制平面服务器")
		return
	}
	switch request.Operation {
	case "install", "upgrade", "restart", "stop", "uninstall", "test":
	default:
		apiShared.BadRequest(c, "Agent 运维操作无效")
		return
	}
	cluster, err := h.store.GetCluster(clusterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取集群失败")
		return
	}
	if cluster.ControlServerID == nil || *cluster.ControlServerID != request.ServerID {
		apiShared.BadRequest(c, "Agent 只能部署在控制平面服务器")
		return
	}
	server, err := h.store.GetServer(request.ServerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return
	}
	task, err := h.store.StartClusterAgentOperation(clusterID, request.ServerID, request.Operation)
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "Agent 已有运维任务正在执行，或服务器不是该集群控制平面")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群或服务器不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建 Agent 运维任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "Agent 运维任务已创建", apiShared.EnsureRequestID(c))
	go h.runAgentOperation(task)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) Adopt(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	var request struct {
		ServerID uint   `json:"server_id"`
		Role     string `json:"role"`
		Address  string `json:"address"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.ServerID == 0 {
		apiShared.BadRequest(c, "请选择服务器")
		return
	}
	if request.Role != "control-plane" && request.Role != "worker" {
		apiShared.BadRequest(c, "节点角色无效")
		return
	}
	server, err := h.store.GetServer(request.ServerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return
	}
	address := strings.TrimSpace(request.Address)
	if request.Role == "control-plane" {
		if address == "" {
			address = server.Host
		}
		if !validControlAddress(address) {
			apiShared.BadRequest(c, "控制平面连接地址应为 IP 或域名，不含协议和端口")
			return
		}
	}
	task, err := h.store.StartClusterAdoption(clusterID, request.ServerID, request.Role, address)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群或服务器不存在")
		return
	}
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "服务器已被占用、集群状态不允许纳管，或已有任务正在进行")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建纳管任务失败")
		return
	}
	_ = h.recordAudit(task, model.AuditOutcomeAccepted, "K3s 纳管任务已创建", apiShared.EnsureRequestID(c))
	go h.runAdoption(task)
	apiShared.Success(c, task)
}

func (h *DeploymentHandler) runAdoption(task *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	log("开始核验已有 " + roleLabel(task.Role))
	token, address, nodeName, err := h.verifyAdoption(ctx, task, log)
	if err != nil {
		log("纳管失败：" + safeDeploymentError(err))
		_ = h.store.FinishClusterDeployment(task, false, "", "", "")
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "K3s 纳管任务失败", "")
		return
	}
	if err := h.store.FinishClusterDeployment(task, true, token, address, nodeName); err != nil {
		log("保存纳管结果失败：" + safeDeploymentError(err))
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存 K3s 纳管结果失败", "")
		return
	}
	log("纳管成功，未修改目标机器上的 K3s 安装")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "K3s 纳管任务成功", "")
}

func (h *DeploymentHandler) verifyAdoption(ctx context.Context, task *model.ClusterDeployment, log func(string)) (token, address, nodeName string, err error) {
	server, err := h.store.GetServer(task.ServerID)
	if err != nil {
		return "", "", "", err
	}
	service := "k3s"
	if task.Role == "worker" {
		service = "k3s-agent"
	}
	log("检查 SSH 与 " + service + " 服务状态")
	output, err := transport.SSHExecServerContext(ctx, 20*time.Second, server, h.key, "systemctl is-active "+service)
	if err != nil || strings.TrimSpace(string(output)) != "active" {
		return "", "", "", fmt.Errorf("目标机器上的 %s 服务未运行或 SSH 不可用", service)
	}
	output, err = transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "hostname")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", "", "", fmt.Errorf("读取节点主机名失败")
	}
	hostname := strings.TrimSpace(string(output))
	control := server
	if task.Role == "worker" {
		cluster, getErr := h.store.GetCluster(task.ClusterID)
		if getErr != nil || cluster.ControlServerID == nil {
			return "", "", "", fmt.Errorf("读取控制平面失败")
		}
		control, err = h.store.GetServer(*cluster.ControlServerID)
		if err != nil {
			return "", "", "", fmt.Errorf("读取控制平面服务器失败")
		}
	} else {
		address = task.Address
	}
	log("从控制平面核对节点是否属于同一 K3s 集群")
	command := "if [ \"$(id -u)\" = 0 ]; then k3s kubectl get nodes -o json; else sudo -n k3s kubectl get nodes -o json; fi"
	output, err = transport.SSHExecServerContext(ctx, 30*time.Second, control, h.key, command)
	if err != nil {
		return "", "", "", fmt.Errorf("无法从控制平面读取 K3s 节点列表")
	}
	var nodes struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Addresses []struct {
					Type    string `json:"type"`
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &nodes); err != nil {
		return "", "", "", fmt.Errorf("解析 K3s 节点列表失败")
	}
	for _, node := range nodes.Items {
		if strings.EqualFold(node.Metadata.Name, hostname) {
			nodeName = node.Metadata.Name
			break
		}
		for _, item := range node.Status.Addresses {
			if (item.Type == "InternalIP" || item.Type == "Hostname") && strings.EqualFold(item.Address, server.Host) {
				nodeName = node.Metadata.Name
				break
			}
		}
		if nodeName != "" {
			break
		}
	}
	if nodeName == "" {
		return "", "", "", fmt.Errorf("控制平面节点列表中找不到该服务器；请核对主机名或节点地址")
	}
	log("已确认节点 " + nodeName)
	if task.Role == "control-plane" {
		log("读取并加密保存集群加入凭据")
		output, err = transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "if [ \"$(id -u)\" = 0 ]; then cat /var/lib/rancher/k3s/server/node-token; else sudo -n cat /var/lib/rancher/k3s/server/node-token; fi")
		if err != nil || strings.TrimSpace(string(output)) == "" {
			return "", "", "", fmt.Errorf("读取 K3s 加入凭据失败")
		}
		token, err = security.Encrypt(h.key, strings.TrimSpace(string(output)))
		if err != nil {
			return "", "", "", fmt.Errorf("加密 K3s 加入凭据失败")
		}
	}
	return token, address, nodeName, nil
}

func (h *DeploymentHandler) runUninstall(task *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	log("开始卸载 " + roleLabel(task.Role))
	server, err := h.store.GetServer(task.ServerID)
	if err == nil {
		script := "/usr/local/bin/k3s-uninstall.sh"
		if task.Role == "worker" {
			script = "/usr/local/bin/k3s-agent-uninstall.sh"
		}
		// The official K3s uninstall script removes the service and local data.
		command := "if [ \"$(id -u)\" = 0 ]; then " + shellEscape(script) + "; else sudo -n " + shellEscape(script) + "; fi"
		log("通过 SSH 执行 K3s 卸载脚本")
		err = h.streamInstall(ctx, server, command, "", log)
	}
	if err != nil {
		log("卸载失败：" + safeDeploymentError(err))
		_ = h.store.FinishClusterUninstall(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "节点卸载任务失败", "")
		return
	}
	if err := h.store.FinishClusterUninstall(task, true); err != nil {
		log("保存卸载结果失败：" + safeDeploymentError(err))
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "保存节点卸载结果失败", "")
		return
	}
	log("卸载完成，服务器已解除集群绑定")
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "节点卸载任务成功", "")
}

type DeploymentHandler struct {
	store           clusterDeploymentStore
	key             []byte
	agentIssuer     agentCertificateIssuer
	agentBinaryPath string
}

func NewDeploymentHandler(st clusterDeploymentStore, key []byte) *DeploymentHandler {
	return &DeploymentHandler{store: st, key: append([]byte(nil), key...), agentBinaryPath: os.Getenv("CYLISM_CLUSTER_AGENT_BINARY")}
}

func (h *DeploymentHandler) WithAgentCertificateIssuer(issuer agentCertificateIssuer) *DeploymentHandler {
	h.agentIssuer = issuer
	return h
}
func (h *DeploymentHandler) WithAgentBinaryPath(path string) *DeploymentHandler {
	h.agentBinaryPath = path
	return h
}

func (h *DeploymentHandler) Start(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	var request struct {
		ServerID uint   `json:"server_id"`
		Role     string `json:"role"`
		Mirror   string `json:"mirror"`
		Address  string `json:"address"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.ServerID == 0 {
		apiShared.BadRequest(c, "请选择服务器")
		return
	}
	if request.Role != "control-plane" && request.Role != "worker" {
		apiShared.BadRequest(c, "节点角色无效")
		return
	}
	if request.Mirror != "official" && request.Mirror != "cn" {
		apiShared.BadRequest(c, "安装源无效")
		return
	}
	if _, err := h.store.GetCluster(clusterID); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	} else if err != nil {
		apiShared.InternalError(c, "读取集群失败")
		return
	}
	server, err := h.store.GetServer(request.ServerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "服务器不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取服务器失败")
		return
	}
	if server.Host == "" || (server.SSHAuthType == "key" && server.SSHKey == "") || (server.SSHAuthType != "key" && server.SSHPassword == "") {
		apiShared.BadRequest(c, "请先补全服务器 SSH 信息")
		return
	}
	address := strings.TrimSpace(request.Address)
	if request.Role == "control-plane" {
		if address == "" {
			address = server.Host
		}
		if !validControlAddress(address) {
			apiShared.BadRequest(c, "控制平面连接地址应为 IP 或域名，不含协议和端口")
			return
		}
	}
	deployment, err := h.store.StartClusterDeployment(clusterID, request.ServerID, request.Role, request.Mirror, address)
	if errors.Is(err, store.ErrClusterDeploymentConflict) {
		apiShared.Conflict(c, "服务器已被占用、集群状态不允许部署，或已有部署正在进行")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "创建部署任务失败")
		return
	}
	_ = h.recordAudit(deployment, model.AuditOutcomeAccepted, "集群部署任务已创建", apiShared.EnsureRequestID(c))
	go h.run(deployment)
	apiShared.Success(c, deployment)
}

func (h *DeploymentHandler) List(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	if _, err := h.store.GetCluster(clusterID); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	} else if err != nil {
		apiShared.InternalError(c, "读取集群失败")
		return
	}
	items, err := h.store.ListClusterDeployments(clusterID)
	if err != nil {
		apiShared.InternalError(c, "读取部署记录失败")
		return
	}
	apiShared.Success(c, items)
}

func (h *DeploymentHandler) Logs(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	deploymentID, err := apiShared.ParsePositiveID(c.Param("deploymentID"))
	if err != nil {
		apiShared.BadRequest(c, "部署记录 ID 无效")
		return
	}
	if _, err := h.store.GetClusterDeployment(clusterID, deploymentID); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "部署记录不存在")
		return
	} else if err != nil {
		apiShared.InternalError(c, "读取部署记录失败")
		return
	}
	after, _ := strconv.ParseUint(c.Query("after"), 10, 32)
	items, err := h.store.ListClusterDeploymentLogs(deploymentID, uint(after))
	if err != nil {
		apiShared.InternalError(c, "读取部署日志失败")
		return
	}
	apiShared.Success(c, items)
}

func (h *DeploymentHandler) run(deployment *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := h.store.AppendClusterDeploymentLog(deployment.ID, "开始部署 "+roleLabel(deployment.Role)); err != nil {
		_ = h.store.FinishClusterDeployment(deployment, false, "", "", "")
		_ = h.recordAudit(deployment, model.AuditOutcomeFailed, "集群部署任务失败：无法写入执行日志", "")
		return
	}
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(deployment.ID, message) }
	token, address, nodeName, err := h.install(ctx, deployment, log)
	if err != nil {
		log("部署失败：" + safeDeploymentError(err))
		_ = h.store.FinishClusterDeployment(deployment, false, "", "", "")
		_ = h.recordAudit(deployment, model.AuditOutcomeFailed, "集群部署任务失败", "")
		return
	}
	log("安装成功，正在保存集群状态")
	if err := h.store.FinishClusterDeployment(deployment, true, token, address, nodeName); err != nil {
		log("保存部署结果失败：" + safeDeploymentError(err))
		_ = h.recordAudit(deployment, model.AuditOutcomeFailed, "保存集群部署结果失败", "")
		return
	}
	_ = h.recordAudit(deployment, model.AuditOutcomeSucceeded, "集群部署任务成功", "")
}

func (h *DeploymentHandler) runAgentOperation(task *model.ClusterDeployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	log := func(message string) { _ = h.store.AppendClusterDeploymentLog(task.ID, message) }
	log("开始执行 Agent 操作：" + agentOperationLabel(task.Operation))
	err := h.executeAgentOperation(ctx, task, log)
	if err != nil {
		log("Agent 操作失败：" + safeDeploymentError(err))
		_ = h.store.FinishClusterAgentOperation(task, false)
		_ = h.recordAudit(task, model.AuditOutcomeFailed, "Agent 运维任务失败", "")
		return
	}
	log("Agent 操作完成")
	_ = h.store.FinishClusterAgentOperation(task, true)
	_ = h.recordAudit(task, model.AuditOutcomeSucceeded, "Agent 运维任务成功", "")
}

func (h *DeploymentHandler) executeAgentOperation(ctx context.Context, task *model.ClusterDeployment, log func(string)) error {
	server, err := h.store.GetServer(task.ServerID)
	if err != nil {
		return err
	}
	cluster, err := h.store.GetCluster(task.ClusterID)
	if err != nil {
		return err
	}
	if task.Operation == "agent-install" || task.Operation == "agent-upgrade" {
		if strings.TrimSpace(h.agentBinaryPath) == "" {
			return fmt.Errorf("未配置 cluster-agent 构建产物，请设置 CYLISM_CLUSTER_AGENT_BINARY")
		}
		binary, err := os.ReadFile(h.agentBinaryPath)
		if err != nil {
			return fmt.Errorf("读取 cluster-agent 构建产物失败")
		}
		const remoteBinary = "/usr/local/bin/cylism-cluster-agent"
		log("上传 cluster-agent 程序")
		if err := uploadAgentBinary(ctx, server, h.key, remoteBinary, binary); err != nil {
			return err
		}
		// Upgrades replace only the executable and restart the existing
		// service. The Agent key and certificate stay on the control plane.
		if task.Operation == "agent-upgrade" {
			command := "if [ \"$(id -u)\" = 0 ]; then systemctl restart cylism-cluster-agent; else sudo -n systemctl restart cylism-cluster-agent; fi"
			if _, err := transport.SSHExecServerContext(ctx, 60*time.Second, server, h.key, command); err != nil {
				return fmt.Errorf("升级后重启 Agent 失败")
			}
			log("Agent 已升级并重启，继续使用原有证书")
			return nil
		}
		if h.agentIssuer == nil {
			return fmt.Errorf("平台访问证书服务未就绪")
		}
		const configDir = "/etc/cylism/cluster-agent"
		if _, err := transport.SSHExecServerContext(ctx, 30*time.Second, server, h.key, "if [ \"$(id -u)\" = 0 ]; then mkdir -p "+configDir+"; else sudo -n mkdir -p "+configDir+"; fi"); err != nil {
			return fmt.Errorf("创建 Agent 配置目录失败")
		}
		log("在控制平面生成 Agent 私钥和 CSR")
		agentHostname := strings.TrimSpace(cluster.ControlAddress)
		if agentHostname == "" {
			agentHostname = server.Host
		}
		csrCommand := remoteBinary + " csr --hostname " + shellQuote(agentHostname) + " --key-path " + configDir + "/agent.key --csr-path " + configDir + "/agent.csr"
		csrOutput, err := transport.SSHExecServerContext(ctx, 30*time.Second, server, h.key, "if [ \"$(id -u)\" = 0 ]; then "+csrCommand+"; else sudo -n "+csrCommand+"; fi")
		if err != nil {
			return fmt.Errorf("生成 Agent CSR 失败")
		}
		cert, err := h.agentIssuer.Issue(task.ClusterID, "agent_server", string(csrOutput))
		if err != nil {
			return fmt.Errorf("签发 Agent 服务端证书失败")
		}
		ca, err := h.agentIssuer.GetCA()
		if err != nil {
			return fmt.Errorf("读取平台访问 CA 失败")
		}
		if err := writeAgentFile(ctx, server, h.key, configDir+"/agent.crt", cert.CertificatePEM, 0600); err != nil {
			return fmt.Errorf("写入 Agent 服务端证书失败")
		}
		if err := writeAgentFile(ctx, server, h.key, configDir+"/ca.crt", ca.CertificatePEM, 0644); err != nil {
			return fmt.Errorf("写入 Agent CA 证书失败")
		}
		unit := "[Unit]\nDescription=Cylism Cluster Agent\nAfter=network-online.target\n[Service]\nExecStart=" + remoteBinary + " serve --cluster-id=" + fmt.Sprint(task.ClusterID) + " --listen-address=0.0.0.0:9443 --kube-api=https://127.0.0.1:6443 --kubeconfig=/etc/rancher/k3s/k3s.yaml --server-cert=" + configDir + "/agent.crt --server-key=" + configDir + "/agent.key --client-ca=" + configDir + "/ca.crt\nRestart=on-failure\nRestartSec=3\n[Install]\nWantedBy=multi-user.target\n"
		if err := writeAgentFile(ctx, server, h.key, "/etc/systemd/system/cylism-cluster-agent.service", unit, 0644); err != nil {
			return fmt.Errorf("写入 Agent 服务配置失败")
		}
		// enable --now does not restart an already running service. Always
		// reload the unit and restart it so upgrades take effect immediately.
		command := "if [ \"$(id -u)\" = 0 ]; then systemctl daemon-reload && systemctl enable cylism-cluster-agent && systemctl restart cylism-cluster-agent; else sudo -n systemctl daemon-reload && sudo -n systemctl enable cylism-cluster-agent && sudo -n systemctl restart cylism-cluster-agent; fi"
		if _, err := transport.SSHExecServerContext(ctx, 60*time.Second, server, h.key, command); err != nil {
			return fmt.Errorf("启动 Agent 服务失败")
		}
		if err := h.agentIssuer.MarkInUse(cert.ID, true); err != nil {
			return fmt.Errorf("标记 Agent 证书状态失败")
		}
		log("Agent 服务已启动")
		return nil
	}
	var command string
	switch task.Operation {
	case "agent-restart":
		command = "systemctl restart cylism-cluster-agent"
	case "agent-stop":
		command = "systemctl stop cylism-cluster-agent"
	case "agent-uninstall":
		command = "systemctl disable --now cylism-cluster-agent 2>/dev/null || true; rm -f /etc/systemd/system/cylism-cluster-agent.service /usr/local/bin/cylism-cluster-agent; rm -rf /etc/cylism/cluster-agent"
	case "agent-test":
		command = "curl -kfsS --max-time 10 https://127.0.0.1:9443/healthz"
	default:
		return fmt.Errorf("不支持的 Agent 操作")
	}
	if _, err := transport.SSHExecServerContext(ctx, 60*time.Second, server, h.key, "if [ \"$(id -u)\" = 0 ]; then "+command+"; else sudo -n sh -c "+shellQuote(command)+"; fi"); err != nil {
		return fmt.Errorf("执行 Agent 操作失败")
	}
	if task.Operation == "agent-uninstall" && h.agentIssuer != nil {
		if err := h.agentIssuer.MarkClusterUnused(task.ClusterID, "agent_server"); err != nil {
			return fmt.Errorf("标记 Agent 证书状态失败")
		}
	}
	return nil
}

func uploadAgentBinary(ctx context.Context, server *model.Server, key []byte, path string, data []byte) error {
	// Never write directly to the running executable. Linux rejects that with
	// ETXTBSY ("Text file busy"); upload beside it and atomically rename it.
	temporaryPath := path + ".uploading"
	quotedPath := shellQuote(path)
	quotedTemporaryPath := shellQuote(temporaryPath)
	commandText := "if [ \"$(id -u)\" = 0 ]; then rm -f " + quotedTemporaryPath + " && cat > " + quotedTemporaryPath + " && chmod 0755 " + quotedTemporaryPath + " && mv -f " + quotedTemporaryPath + " " + quotedPath + "; else sudo -n rm -f " + quotedTemporaryPath + " && sudo -n tee " + quotedTemporaryPath + " >/dev/null && sudo -n chmod 0755 " + quotedTemporaryPath + " && sudo -n mv -f " + quotedTemporaryPath + " " + quotedPath + "; fi"
	command, err := transport.StartSSHCommandContext(ctx, 2*time.Minute, server, key, commandText)
	if err != nil {
		return fmt.Errorf("连接服务器并上传 Agent 程序失败：%w", err)
	}
	var stdout, stderr bytes.Buffer
	var streams sync.WaitGroup
	streams.Add(2)
	go func() {
		defer streams.Done()
		_, _ = io.Copy(&stdout, command.Stdout)
	}()
	go func() {
		defer streams.Done()
		_, _ = io.Copy(&stderr, command.Stderr)
	}()
	if _, err := command.Stdin.Write(data); err != nil {
		command.Close()
		streams.Wait()
		return fmt.Errorf("向服务器传输 Agent 程序失败：%w", err)
	}
	_ = command.Stdin.Close()
	waitErr := command.Wait()
	streams.Wait()
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			if len(detail) > 2000 {
				detail = detail[:2000] + "…"
			}
			return fmt.Errorf("上传 Agent 程序失败：%s (%w)", detail, waitErr)
		}
		return fmt.Errorf("上传 Agent 程序失败：%w", waitErr)
	}
	return nil
}

func writeAgentFile(ctx context.Context, server *model.Server, key []byte, path, content string, mode int) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	command := "echo " + shellQuote(encoded) + " | base64 -d | "
	if mode == 0600 {
		command += "(if [ \"$(id -u)\" = 0 ]; then cat > " + path + "; else sudo -n tee " + path + " >/dev/null; fi)"
	} else {
		command += "(if [ \"$(id -u)\" = 0 ]; then cat > " + path + "; else sudo -n tee " + path + " >/dev/null; fi)"
	}
	command += " && (if [ \"$(id -u)\" = 0 ]; then chmod " + fmt.Sprintf("%04o", mode) + " " + path + "; else sudo -n chmod " + fmt.Sprintf("%04o", mode) + " " + path + "; fi)"
	_, err := transport.SSHExecServerContext(ctx, 60*time.Second, server, key, command)
	return err
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func agentOperationLabel(operation string) string {
	return map[string]string{"agent-install": "部署 Agent", "agent-upgrade": "升级 Agent", "agent-restart": "重启 Agent", "agent-stop": "停止 Agent", "agent-uninstall": "卸载 Agent", "agent-test": "测试 Agent 连接"}[operation]
}

func (h *DeploymentHandler) recordAudit(deployment *model.ClusterDeployment, outcome, summary, requestID string) error {
	action := "cluster.deploy"
	if deployment.Operation == "uninstall" {
		action = "cluster.uninstall"
	} else if deployment.Operation == "adopt" {
		action = "cluster.adopt"
	} else if strings.HasPrefix(deployment.Operation, "agent-") {
		action = "cluster.agent"
	}
	return auditservice.NewService(h.store).Record(auditservice.AuditEventInput{
		Action: action, ResourceType: "cluster", ResourceID: deployment.ClusterID,
		Actor:  auditservice.Actor{Type: model.AuditActorSystem},
		Source: model.AuditSourceAPI, Outcome: outcome, Summary: summary,
		RequestID: requestID, OperationID: fmt.Sprintf("cluster-deployment-%d", deployment.ID),
		Metadata: map[string]any{"deployment_id": deployment.ID, "server_id": deployment.ServerID, "role": deployment.Role, "mirror": deployment.Mirror, "operation": deployment.Operation},
	})
}

func (h *DeploymentHandler) install(ctx context.Context, deployment *model.ClusterDeployment, log func(string)) (encryptedToken, address, nodeName string, err error) {
	server, err := h.store.GetServer(deployment.ServerID)
	if err != nil {
		return "", "", "", err
	}
	cluster, err := h.store.GetCluster(deployment.ClusterID)
	if err != nil {
		return "", "", "", err
	}
	var plainToken string
	if deployment.Role == "worker" {
		plainToken, err = security.Decrypt(h.key, cluster.EncryptedToken)
		if err != nil {
			return "", "", "", fmt.Errorf("读取集群加入凭据失败")
		}
		address = cluster.ControlAddress
		if address == "" {
			return "", "", "", fmt.Errorf("控制平面地址缺失")
		}
	} else {
		address = deployment.Address
	}
	log("检查 SSH 与安装条件")
	check, err := transport.SSHExecServerContext(ctx, 20*time.Second, server, h.key, "command -v bash && command -v curl && command -v systemctl && { test \"$(id -u)\" = 0 || sudo -n true; }")
	if err != nil {
		return "", "", "", fmt.Errorf("SSH 或安装条件检查失败：%s", strings.TrimSpace(string(check)))
	}
	service := "k3s"
	if deployment.Role == "worker" {
		service = "k3s-agent"
	}
	serviceFile := "/etc/systemd/system/" + service + ".service"
	check, err = transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "test ! -e "+shellEscape(serviceFile))
	if err != nil {
		return "", "", "", fmt.Errorf("目标机器存在 %s 安装记录，请先核对或清理", service)
	}
	check, _ = transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "systemctl is-active "+service+" 2>/dev/null || true")
	if strings.TrimSpace(string(check)) == "active" {
		return "", "", "", fmt.Errorf("目标机器上 %s 已运行，请先核对现有集群", service)
	}
	command := k3sInstallCommand(deployment.Role, deployment.Mirror, address, plainToken)
	log("从" + mirrorLabel(deployment.Mirror) + "下载安装 K3s")
	if err := h.streamInstall(ctx, server, command, plainToken, log); err != nil {
		return "", "", "", err
	}
	log("等待 " + service + " 服务启动")
	ready := false
	for attempt := 0; attempt < 30; attempt++ {
		out, runErr := transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "systemctl is-active "+service)
		if runErr == nil && strings.TrimSpace(string(out)) == "active" {
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			return "", "", "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if !ready {
		return "", "", "", fmt.Errorf("等待 %s 服务启动超时", service)
	}
	out, err := transport.SSHExecServerContext(ctx, 10*time.Second, server, h.key, "hostname")
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "", "", "", fmt.Errorf("读取节点主机名失败")
	}
	nodeName = strings.TrimSpace(string(out))
	if deployment.Role == "control-plane" {
		log("读取控制平面加入凭据")
		out, err = transport.SSHExecServerContext(ctx, 15*time.Second, server, h.key, "if [ \"$(id -u)\" = 0 ]; then cat /var/lib/rancher/k3s/server/node-token; else sudo -n cat /var/lib/rancher/k3s/server/node-token; fi")
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return "", "", "", fmt.Errorf("读取 K3s 加入凭据失败")
		}
		encryptedToken, err = security.Encrypt(h.key, strings.TrimSpace(string(out)))
		if err != nil {
			return "", "", "", fmt.Errorf("加密 K3s 加入凭据失败")
		}
	}
	return encryptedToken, address, nodeName, nil
}

func (h *DeploymentHandler) streamInstall(ctx context.Context, server *model.Server, command, token string, log func(string)) error {
	run, err := transport.StartSSHCommandContext(ctx, 20*time.Minute, server, h.key, command)
	if err != nil {
		return fmt.Errorf("启动远程安装失败")
	}
	defer run.Close()
	var wg sync.WaitGroup
	var logMu sync.Mutex
	readErrors := make(chan error, 2)
	read := func(source io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if token != "" {
				line = strings.ReplaceAll(line, token, "[REDACTED]")
			}
			if len(line) > 2000 {
				line = line[:2000] + "…"
			}
			logMu.Lock()
			log(line)
			logMu.Unlock()
		}
		if scanErr := scanner.Err(); scanErr != nil {
			readErrors <- scanErr
		}
	}
	wg.Add(2)
	go read(run.Stdout)
	go read(run.Stderr)
	wg.Wait()
	err = run.Wait()
	if err != nil {
		return fmt.Errorf("远程命令执行失败：%w", err)
	}
	select {
	case <-readErrors:
		return fmt.Errorf("读取远程安装日志失败")
	default:
	}
	return nil
}

func k3sInstallCommand(role, mirror, address, token string) string {
	url := "https://get.k3s.io"
	variables := ""
	if mirror == "cn" {
		url = "https://rancher-mirror.rancher.cn/k3s/k3s-install.sh"
		variables = "INSTALL_K3S_MIRROR=cn INSTALL_K3S_SKIP_SELINUX_RPM=true "
	}
	if role == "worker" {
		variables += "K3S_URL=" + shellEscape("https://"+net.JoinHostPort(address, "6443")) + " K3S_TOKEN=" + shellEscape(token) + " "
	} else {
		variables += "INSTALL_K3S_EXEC=" + shellEscape("server --tls-san "+address) + " "
	}
	pipeline := "curl -sfL " + url + " | " + variables + "sh -"
	return "if [ \"$(id -u)\" = 0 ]; then bash -o pipefail -c " + shellEscape(pipeline) + "; else bash -o pipefail -c " + shellEscape("curl -sfL "+url+" | sudo -n env "+variables+"sh -") + "; fi"
}

func roleLabel(role string) string {
	if role == "worker" {
		return "工作节点"
	}
	return "控制平面"
}
func mirrorLabel(mirror string) string {
	if mirror == "cn" {
		return "国内镜像"
	}
	return "官方源"
}
func safeDeploymentError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func validControlAddress(address string) bool {
	if net.ParseIP(address) != nil {
		return true
	}
	if len(address) == 0 || len(address) > 253 {
		return false
	}
	for _, label := range strings.Split(address, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}
