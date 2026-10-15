package cluster

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

type singBoxStatus struct {
	ServerID   uint   `json:"server_id"`
	Installed  bool   `json:"installed"`
	Configured bool   `json:"configured"`
	Running    bool   `json:"running"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

const singBoxStatusMarker = "__CYLISM_SING_BOX_STATUS__"

func inspectSingBox(ctx context.Context, server *model.Server, key []byte) singBoxStatus {
	status := singBoxStatus{ServerID: server.ID}
	command := `printf '%s\n' '` + singBoxStatusMarker + `'; ` +
		`if ! command -v sing-box >/dev/null 2>&1; then echo 'installed=0'; exit 0; fi; ` +
		`echo 'installed=1'; sing-box version | head -n 1; ` +
		`if test -s /etc/sing-box/config.json; then echo 'configured=1'; fi; ` +
		`if systemctl is-active --quiet sing-box; then echo 'running=1'; fi`
	output, err := transport.SSHExecServerContext(ctx, 12*time.Second, server, key, privilegedCommand(server, command))
	if err != nil {
		status.Error = "无法通过 SSH 读取 sing-box 状态，请检查连接和 sudo 权限"
		return status
	}
	_, payload, found := bytes.Cut(output, []byte(singBoxStatusMarker+"\n"))
	if !found {
		status.Error = "服务器未返回 sing-box 状态，请检查 SSH 登录脚本"
		return status
	}
	for _, line := range strings.Split(string(payload), "\n") {
		switch strings.TrimSpace(line) {
		case "installed=1":
			status.Installed = true
		case "configured=1":
			status.Configured = true
		case "running=1":
			status.Running = true
		default:
			if strings.HasPrefix(line, "sing-box version ") {
				status.Version = strings.TrimPrefix(strings.TrimSpace(line), "sing-box version ")
			}
		}
	}
	return status
}

func (h *ServerHandler) SingBoxStatus(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if ok {
		apiShared.Success(c, inspectSingBox(c.Request.Context(), server, h.encKey))
	}
}

func (h *ServerHandler) SingBoxInstall(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	var request struct {
		EgressServerID *uint `json:"egress_server_id"`
	}
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&request) != nil {
		apiShared.BadRequest(c, "安装参数无效")
		return
	}
	if request.EgressServerID != nil && (*request.EgressServerID == 0 || *request.EgressServerID == server.ID) {
		apiShared.BadRequest(c, "请选择其他服务器作为下载出口")
		return
	}
	egressID := uint(0)
	if request.EgressServerID != nil {
		egressID = *request.EgressServerID
	}
	log.Printf("[sing-box-install] target_id=%d egress_id=%d request received", server.ID, egressID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Minute)
	defer cancel()
	proxyURL := ""
	var proxyTrace *transport.SSHDownloadProxyTrace
	if request.EgressServerID != nil {
		egress, err := h.cluster.GetServer(*request.EgressServerID)
		if err != nil || egress.ProxyServerID != nil {
			apiShared.BadRequest(c, "下载出口节点不存在或不能直接访问")
			return
		}
		var closeProxy func()
		proxyTrace = &transport.SSHDownloadProxyTrace{}
		proxyURL, closeProxy, err = transport.StartSSHDownloadProxyContext(ctx, server, egress, h.encKey, proxyTrace)
		if err != nil {
			log.Printf("[sing-box-install] target_id=%d egress_id=%d proxy setup failed: %v", server.ID, *request.EgressServerID, err)
			apiShared.BadRequest(c, "建立下载代理失败："+err.Error())
			return
		}
		defer closeProxy()
	}
	command := `set -eu; if command -v sing-box >/dev/null 2>&1; then exit 0; fi; ` +
		`curl -fsSL --connect-timeout 10 --max-time 90 --proto '=https' https://sing-box.app/install.sh | sh; ` +
		`command -v sing-box >/dev/null 2>&1; systemctl disable --now sing-box >/dev/null 2>&1 || true`
	if proxyURL != "" {
		command = "export http_proxy=" + shellQuote(proxyURL) + " https_proxy=" + shellQuote(proxyURL) +
			" HTTP_PROXY=" + shellQuote(proxyURL) + " HTTPS_PROXY=" + shellQuote(proxyURL) + "; " + command
	}
	output, err := transport.SSHExecServerContext(ctx, 4*time.Minute, server, h.encKey, privilegedCommand(server, "bash -o pipefail -c "+shellQuote(command)))
	if err != nil {
		message := "安装 sing-box 失败：" + trimCommandOutput(output)
		if ctx.Err() == context.DeadlineExceeded {
			message = "安装 sing-box 超过 4 分钟：" + trimCommandOutput(output)
		}
		if proxyTrace != nil {
			message += "；下载通道：" + proxyTrace.Stage()
		}
		log.Printf("[sing-box-install] target_id=%d command failed: %v; proxy_stage=%q", server.ID, err, proxyTrace.Stage())
		apiShared.BadRequest(c, message)
		return
	}
	status := inspectSingBox(ctx, server, h.encKey)
	if !status.Installed || status.Error != "" {
		apiShared.BadRequest(c, "安装完成后未能确认 sing-box 状态："+status.Error)
		return
	}
	apiShared.Success(c, status)
}

func validSingBoxURL(raw string) bool {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return true
}

func singBoxImportScript(configURL string) string {
	return `set -eu
umask 077
url=` + shellQuote(configURL) + `
if ! command -v sing-box >/dev/null 2>&1; then echo NOT_INSTALLED; exit 1; fi
mkdir -p -m 700 /etc/sing-box
cd /etc/sing-box
tmp=$(mktemp /etc/sing-box/.cylism-config.XXXXXX)
backup=$(mktemp /etc/sing-box/.cylism-backup.XXXXXX)
trap 'rm -f "$tmp" "$backup"' EXIT
if ! curl -fLsS --connect-timeout 10 --max-time 45 --max-redirs 3 --max-filesize 2097152 --proto '=https' --proto-redir '=https' -o "$tmp" "$url" 2>/dev/null; then echo DOWNLOAD_FAILED; exit 1; fi
if ! sing-box check -c "$tmp" >/dev/null 2>&1; then echo INVALID_CONFIG; exit 1; fi
had_config=0
if test -e /etc/sing-box/config.json; then cp -p /etc/sing-box/config.json "$backup"; had_config=1; fi
chmod 600 "$tmp"
mv "$tmp" /etc/sing-box/config.json
if systemctl is-active --quiet sing-box; then
  if ! systemctl restart sing-box >/dev/null 2>&1 || ! systemctl is-active --quiet sing-box; then
    if test "$had_config" = 1; then cp -p "$backup" /etc/sing-box/config.json; else rm -f /etc/sing-box/config.json; fi
    systemctl restart sing-box >/dev/null 2>&1 || true
    echo RESTART_FAILED
    exit 1
  fi
fi
echo OK
`
}

func runSingBoxImport(ctx context.Context, server *model.Server, key []byte, configURL string) (string, error) {
	command := "sh -s"
	if server.SSHUser != "" && server.SSHUser != "root" {
		command = "sudo -n sh -s"
	}
	run, err := transport.StartSSHCommandContext(ctx, 75*time.Second, server, key, command)
	if err != nil {
		return "", err
	}
	defer run.Close()
	var output bytes.Buffer
	var reads sync.WaitGroup
	reads.Add(2)
	go func() { defer reads.Done(); _, _ = io.Copy(io.Discard, run.Stderr) }()
	go func() {
		defer reads.Done()
		_, _ = io.Copy(&output, io.LimitReader(run.Stdout, 8192))
		_, _ = io.Copy(io.Discard, run.Stdout)
	}()
	_, writeErr := io.WriteString(run.Stdin, singBoxImportScript(configURL))
	_ = run.Stdin.Close()
	reads.Wait()
	if writeErr != nil {
		return "", writeErr
	}
	result := ""
	for _, line := range strings.Split(output.String(), "\n") {
		switch strings.TrimSpace(line) {
		case "OK", "NOT_INSTALLED", "DOWNLOAD_FAILED", "INVALID_CONFIG", "RESTART_FAILED":
			result = strings.TrimSpace(line)
		}
	}
	return result, run.Wait()
}

func (h *ServerHandler) SingBoxImport(c *gin.Context) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	var request struct {
		URL string `json:"url"`
	}
	if c.ShouldBindJSON(&request) != nil || !validSingBoxURL(request.URL) {
		apiShared.BadRequest(c, "请输入有效的 HTTPS sing-box JSON 配置地址")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 75*time.Second)
	defer cancel()
	result, err := runSingBoxImport(ctx, server, h.encKey, request.URL)
	if err != nil || result != "OK" {
		messages := map[string]string{
			"NOT_INSTALLED":   "目标服务器尚未安装 sing-box",
			"DOWNLOAD_FAILED": "目标服务器无法下载配置，请检查地址和网络连接",
			"INVALID_CONFIG":  "下载的内容不是有效的 sing-box 配置",
			"RESTART_FAILED":  "新配置启动失败，已恢复上一份配置",
		}
		message := messages[result]
		if message == "" {
			message = "导入配置失败，请检查目标服务器的 SSH 和 sudo 权限"
		}
		apiShared.BadRequest(c, message)
		return
	}
	apiShared.Success(c, inspectSingBox(ctx, server, h.encKey))
}

func (h *ServerHandler) SingBoxStart(c *gin.Context) { h.singBoxToggle(c, true) }
func (h *ServerHandler) SingBoxStop(c *gin.Context)  { h.singBoxToggle(c, false) }

func (h *ServerHandler) singBoxToggle(c *gin.Context, start bool) {
	server, ok := h.tailscaleServer(c)
	if !ok {
		return
	}
	command := "systemctl disable --now sing-box"
	if start {
		command = `test -s /etc/sing-box/config.json && sing-box check -c /etc/sing-box/config.json >/dev/null 2>&1 && systemctl enable --now sing-box && systemctl is-active --quiet sing-box`
	}
	output, err := transport.SSHExecServerContext(c.Request.Context(), 30*time.Second, server, h.encKey, privilegedCommand(server, command))
	if err != nil {
		message := "停止 sing-box 失败"
		if start {
			message = "启动 sing-box 失败，请确认已导入有效配置及 systemd 服务可用"
		}
		apiShared.BadRequest(c, message+"："+trimCommandOutput(output))
		return
	}
	apiShared.Success(c, inspectSingBox(c.Request.Context(), server, h.encKey))
}
