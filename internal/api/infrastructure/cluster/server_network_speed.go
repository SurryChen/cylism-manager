package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
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
	record := &model.ServerSpeedTest{OwnerID: apiShared.UserID(c), SourceID: result.SourceID, TargetID: result.TargetID, TargetAddress: result.TargetAddress, Address: result.Address, Protocol: result.Protocol, Direction: result.Direction, SenderMbps: result.SenderMbps, ReceiverMbps: result.ReceiverMbps, Retransmits: result.Retransmits, LostPackets: result.LostPackets, TotalPackets: result.TotalPackets, LossPercent: result.LossPercent}
	if err := h.cluster.SaveServerSpeedTest(record); err != nil {
		apiShared.DBError(c, "测速已完成，但结果保存失败")
		return
	}
	apiShared.Success(c, record)
}

func (h *ServerHandler) SpeedTestHistory(c *gin.Context) {
	limit, offset := apiShared.LimitOffset(c, 10, 100)
	items, total, err := h.cluster.ListServerSpeedTests(apiShared.UserID(c), limit, offset)
	if err != nil {
		apiShared.DBError(c, "读取测速记录失败")
		return
	}
	apiShared.Success(c, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
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
	listener, err := transport.StartSSHCommandContext(ctx, 12*time.Second, target, h.encKey, speedListenerCommand(speedTestPort))
	if err != nil {
		return nil, fmt.Errorf("无法在目标服务器启动测速监听：%w", err)
	}
	listenerDone := make(chan error, 1)
	defer stopSpeedListener(listener, listenerDone)
	startup := newSpeedListenerOutput()
	var streams sync.WaitGroup
	streams.Add(2)
	for _, stream := range []io.Reader{listener.Stdout, listener.Stderr} {
		go func(reader io.Reader) {
			defer streams.Done()
			_, _ = io.Copy(startup, reader)
		}(stream)
	}
	go func() {
		streams.Wait()
		listenerDone <- listener.Wait()
		close(listenerDone)
	}()
	if err := waitSpeedListener(ctx, startup, listenerDone); err != nil {
		return nil, err
	}
	command := fmt.Sprintf("iperf3 -J -c %s -p %d -t 10 --connect-timeout 5000", shellQuote(address), speedTestPort)
	if req.Direction == "reverse" {
		command += " -R"
	}
	if req.Protocol == "udp" {
		command += " -u -b 1M -l 1000"
	}
	command = "sh -c " + shellQuote("export LC_ALL=C; exec "+command)
	output, stderr, err := transport.SSHExecServerOutputContext(ctx, 12*time.Second, source, h.encKey, command)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, reportErr := parseSpeedTestOutput(output, req.Protocol); reportErr != nil && json.Valid(output) {
			return nil, fmt.Errorf("测速失败：%w（目标 %s，控制连接使用 TCP %d，UDP 测速还需要 UDP %d）", reportErr, address, speedTestPort, speedTestPort)
		}
		return nil, fmt.Errorf("测速失败（目标 %s，TCP/UDP %d）：%s", address, speedTestPort, speedCommandDetail(stderr, err))
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

// Keep stdin open for the lifetime of this test. EOF (including SSH disconnect)
// stops only this invocation's timeout supervisor, which forwards TERM to iperf3.
// Never kill by port/name: an existing listener may belong to a system service.
func speedListenerCommand(port int) string {
	script := fmt.Sprintf(`export LC_ALL=C
child=
watcher=
owner=$$
cleanup() {
  trap '' HUP INT TERM
  if [ -n "$watcher" ]; then
    kill -TERM "$watcher" 2>/dev/null || :
    wait "$watcher" 2>/dev/null || :
  fi
  if [ -n "$child" ]; then
    kill -TERM "$child" 2>/dev/null || :
    wait "$child" 2>/dev/null || :
  fi
}
trap cleanup EXIT
trap 'exit 143' HUP INT TERM
exec 3<&0
timeout -k 2s 30s iperf3 -s -1 --forceflush -p %d </dev/null 3<&- &
child=$!
( IFS= read -r stop <&3 || :; kill -TERM "$owner" 2>/dev/null || : ) &
watcher=$!
wait "$child"
status=$?
child=
exit "$status"
`, port)
	return "sh -c " + shellQuote(script)
}

func stopSpeedListener(listener *transport.SSHCommand, done <-chan error) {
	// Allow the remote shell to reap its child before closing the SSH transport.
	_ = listener.Stdin.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	listener.Close()
}

type speedListenerOutput struct {
	mu     sync.Mutex
	output string
	ready  chan struct{}
	failed chan struct{}
}

func newSpeedListenerOutput() *speedListenerOutput {
	return &speedListenerOutput{ready: make(chan struct{}, 1), failed: make(chan struct{}, 1)}
}

func (s *speedListenerOutput) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output += string(p)
	if len(s.output) > 4096 {
		s.output = s.output[len(s.output)-4096:]
	}
	if strings.Contains(s.output, "Server listening on") {
		select {
		case s.ready <- struct{}{}:
		default:
		}
	}
	if strings.Contains(s.output, "iperf3: error") {
		select {
		case s.failed <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func (s *speedListenerOutput) detail(err error) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return speedCommandDetail([]byte(s.output), err)
}

func speedCommandDetail(output []byte, err error) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 1000 {
		message = message[len(message)-1000:]
	}
	if message == "" {
		if err == nil {
			return "未收到远端诊断输出"
		}
		return fmt.Sprint(err)
	}
	return message
}

func waitSpeedListener(ctx context.Context, output *speedListenerOutput, done <-chan error) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-output.failed:
		return fmt.Errorf("目标服务器测速监听启动失败（TCP %d）：%s；若端口被占用，请检查 iperf3.service 等常驻服务", speedTestPort, output.detail(nil))
	case err := <-done:
		return fmt.Errorf("目标服务器测速监听提前退出（TCP %d）：%s", speedTestPort, output.detail(err))
	case <-output.ready:
		return nil
	case <-timer.C:
		return fmt.Errorf("等待目标服务器测速监听就绪超时（TCP %d）：%s", speedTestPort, output.detail(ctx.Err()))
	}
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
