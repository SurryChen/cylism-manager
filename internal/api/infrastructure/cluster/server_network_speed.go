package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

type speedTestRequest struct {
	SourceID  uint   `json:"source_id"`
	TargetID  uint   `json:"target_id"`
	Address   string `json:"address"`
	Protocol  string `json:"protocol"`
	Direction string `json:"direction"`
}

var speedTestSlot = make(chan struct{}, 1)

const speedTestPort = 5201

type speedTestResult struct {
	SourceID      uint    `json:"source_id"`
	TargetID      uint    `json:"target_id"`
	TargetAddress string  `json:"target_address"`
	Address       string  `json:"address"`
	Protocol      string  `json:"protocol"`
	Direction     string  `json:"direction"`
	SenderMbps    float64 `json:"sender_mbps"`
	ReceiverMbps  float64 `json:"receiver_mbps"`
	Retransmits   int     `json:"retransmits"`
	LostPackets   int     `json:"lost_packets"`
	TotalPackets  int     `json:"total_packets"`
	LossPercent   float64 `json:"loss_percent"`
}

func validateSpeedTest(req speedTestRequest) error {
	if req.SourceID == 0 || req.TargetID == 0 || req.SourceID == req.TargetID {
		return fmt.Errorf("请选择两台不同的服务器")
	}
	if req.Address != "registered" && req.Address != "tailscale" {
		return fmt.Errorf("目标地址类型无效")
	}
	if req.Protocol != "tcp" && req.Protocol != "udp" {
		return fmt.Errorf("测速协议无效")
	}
	if req.Direction != "forward" && req.Direction != "reverse" {
		return fmt.Errorf("测速方向无效")
	}
	return nil
}

func (h *ServerHandler) SpeedTest(c *gin.Context) {
	var req speedTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "测速参数无效")
		return
	}
	if err := validateSpeedTest(req); err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	select {
	case speedTestSlot <- struct{}{}:
		defer func() { <-speedTestSlot }()
	default:
		apiShared.Conflict(c, "已有节点测速正在进行，请稍后重试")
		return
	}
	source, err := h.cluster.GetServer(req.SourceID)
	if err != nil {
		apiShared.NotFound(c, "来源服务器不存在")
		return
	}
	target, err := h.cluster.GetServer(req.TargetID)
	if err != nil {
		apiShared.NotFound(c, "目标服务器不存在")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	address := strings.TrimSpace(target.Host)
	if req.Address == "tailscale" {
		status := inspectTailscale(ctx, target, h.encKey)
		if status.Status != "online" || net.ParseIP(status.IP) == nil {
			apiShared.BadRequest(c, "目标服务器的 Tailscale IP 不可用："+status.Error)
			return
		}
		address = status.IP
	}
	result, err := h.runSpeedTest(ctx, source, target, address, req)
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	apiShared.Success(c, result)
}

func (h *ServerHandler) runSpeedTest(ctx context.Context, source, target *model.Server, address string, req speedTestRequest) (*speedTestResult, error) {
	for _, node := range []*model.Server{source, target} {
		status, err := inspectSpeedDependencies(ctx, node, h.encKey)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", node.Name, err)
		}
		if !status.Iperf3Installed || (node.ID == target.ID && !status.TimeoutInstalled) {
			return nil, fmt.Errorf("%s 缺少测速工具，请在网络测速页面安装后重试", node.Name)
		}
	}
	listener, err := transport.StartSSHCommandContext(ctx, 12*time.Second, target, h.encKey, fmt.Sprintf("timeout 30s iperf3 -s -1 -p %d", speedTestPort))
	if err != nil {
		return nil, fmt.Errorf("无法在目标服务器启动测速监听：%w", err)
	}
	defer listener.Close()
	go io.Copy(io.Discard, listener.Stdout)
	go io.Copy(io.Discard, listener.Stderr)
	listenerDone := make(chan error, 1)
	go func() { listenerDone <- listener.Wait() }()
	select {
	case <-listenerDone:
		return nil, fmt.Errorf("目标服务器的测速端口 %d 无法监听，请检查端口是否被占用", speedTestPort)
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	command := fmt.Sprintf("iperf3 -J -c %s -p %d -t 10", shellQuote(address), speedTestPort)
	if req.Direction == "reverse" {
		command += " -R"
	}
	if req.Protocol == "udp" {
		command += " -u -b 1M -l 1000"
	}
	output, _, err := transport.SSHExecServerOutputContext(ctx, 12*time.Second, source, h.encKey, command)
	if err != nil {
		return nil, fmt.Errorf("测速失败：请检查目标地址 %s 的节点间连通性，以及 TCP/UDP %d 的防火墙规则", address, speedTestPort)
	}
	result, err := parseSpeedTestOutput(output, req.Protocol)
	if err != nil {
		return nil, fmt.Errorf("测速结果无效：%w", err)
	}
	result.SourceID, result.TargetID = req.SourceID, req.TargetID
	result.TargetAddress, result.Address = address, req.Address
	result.Protocol, result.Direction = req.Protocol, req.Direction
	return result, nil
}

func parseSpeedTestOutput(output []byte, protocol string) (*speedTestResult, error) {
	var report struct {
		Error string `json:"error"`
		End   struct {
			Sent struct {
				BitsPerSecond float64 `json:"bits_per_second"`
				Retransmits   int     `json:"retransmits"`
			} `json:"sum_sent"`
			Received struct {
				BitsPerSecond *float64 `json:"bits_per_second"`
				LostPackets   int      `json:"lost_packets"`
				Packets       int      `json:"packets"`
				LostPercent   float64  `json:"lost_percent"`
			} `json:"sum_received"`
		} `json:"end"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, err
	}
	if report.Error != "" {
		return nil, fmt.Errorf("iperf3 报告错误：%s", report.Error)
	}
	if report.End.Received.BitsPerSecond == nil {
		return nil, fmt.Errorf("未收到有效传输结果")
	}
	result := &speedTestResult{
		SenderMbps:   report.End.Sent.BitsPerSecond / 1e6,
		ReceiverMbps: *report.End.Received.BitsPerSecond / 1e6,
		Retransmits:  report.End.Sent.Retransmits,
	}
	if protocol == "udp" {
		result.LostPackets = report.End.Received.LostPackets
		result.TotalPackets = report.End.Received.Packets
		result.LossPercent = report.End.Received.LostPercent
	}
	return result, nil
}
