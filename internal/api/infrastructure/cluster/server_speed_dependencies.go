package cluster

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
)

const speedDependencyMarker = "__CYLISM_SPEED_DEPS__"

type speedDependencies struct {
	ServerID         uint `json:"server_id"`
	Iperf3Installed  bool `json:"iperf3_installed"`
	TimeoutInstalled bool `json:"timeout_installed"`
}

func (h *ServerHandler) speedDependenciesServer(c *gin.Context) (*model.Server, bool) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "服务器 ID 无效")
		return nil, false
	}
	server, err := h.cluster.GetServer(id)
	if err != nil {
		apiShared.NotFound(c, "服务器不存在")
		return nil, false
	}
	return server, true
}

func (h *ServerHandler) SpeedDependencies(c *gin.Context) {
	server, ok := h.speedDependenciesServer(c)
	if !ok {
		return
	}
	status, err := inspectSpeedDependencies(c.Request.Context(), server, h.encKey)
	if err != nil {
		apiShared.BadRequest(c, fmt.Sprintf("%s：%v", server.Name, err))
		return
	}
	apiShared.Success(c, status)
}

func inspectSpeedDependencies(ctx context.Context, server *model.Server, key []byte) (speedDependencies, error) {
	command := "printf '" + speedDependencyMarker + "'; " +
		"if command -v iperf3 >/dev/null 2>&1; then printf '1'; else printf '0'; fi; " +
		"if command -v timeout >/dev/null 2>&1; then printf '1'; else printf '0'; fi"
	output, _, err := transport.SSHExecServerOutputContext(ctx, 12*time.Second, server, key, command)
	if err != nil {
		return speedDependencies{}, fmt.Errorf("SSH 检测失败，请检查连接和认证")
	}
	_, values, found := bytes.Cut(output, []byte(speedDependencyMarker))
	if !found || len(values) < 2 || (values[0] != '0' && values[0] != '1') || (values[1] != '0' && values[1] != '1') {
		return speedDependencies{}, fmt.Errorf("远端依赖检测结果无效")
	}
	return speedDependencies{ServerID: server.ID, Iperf3Installed: values[0] == '1', TimeoutInstalled: values[1] == '1'}, nil
}

func (h *ServerHandler) InstallSpeedDependencies(c *gin.Context) {
	server, ok := h.speedDependenciesServer(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	status, err := inspectSpeedDependencies(ctx, server, h.encKey)
	if err != nil {
		apiShared.BadRequest(c, fmt.Sprintf("%s：%v", server.Name, err))
		return
	}
	if status.Iperf3Installed && status.TimeoutInstalled {
		apiShared.Success(c, status)
		return
	}
	command := speedDependencyInstallCommand(status)
	output, stderr, err := transport.SSHExecServerOutputContext(ctx, 170*time.Second, server, h.encKey, command)
	if err != nil {
		message := speedInstallFailure(append(output, stderr...))
		apiShared.BadRequest(c, fmt.Sprintf("%s 安装测速工具失败：%s", server.Name, message))
		return
	}
	status, err = inspectSpeedDependencies(ctx, server, h.encKey)
	if err != nil || !status.Iperf3Installed || !status.TimeoutInstalled {
		apiShared.BadRequest(c, fmt.Sprintf("%s 安装后仍未检测到测速工具，请检查远端包管理器输出", server.Name))
		return
	}
	apiShared.Success(c, status)
}

func speedDependencyInstallCommand(status speedDependencies) string {
	packages := make([]string, 0, 2)
	if !status.Iperf3Installed {
		packages = append(packages, "iperf3")
	}
	if !status.TimeoutInstalled {
		packages = append(packages, "coreutils")
	}
	list := strings.Join(packages, " ")
	script := "set -e; " +
		"if [ \"$(id -u)\" = 0 ]; then privilege=''; " +
		"elif command -v sudo >/dev/null 2>&1 && sudo -n true; then privilege='sudo -n'; " +
		"else echo '需要 root 或免密码 sudo 权限' >&2; exit 1; fi; " +
		"if command -v apt-get >/dev/null 2>&1; then " +
		"$privilege env DEBIAN_FRONTEND=noninteractive apt-get update -qq && " +
		"$privilege env DEBIAN_FRONTEND=noninteractive apt-get install -y " + list + "; " +
		"elif command -v dnf >/dev/null 2>&1; then $privilege dnf install -y " + list + "; " +
		"elif command -v yum >/dev/null 2>&1; then $privilege yum install -y " + list + "; " +
		"elif command -v apk >/dev/null 2>&1; then $privilege apk add --no-cache " + list + "; " +
		"else echo '不支持的包管理器' >&2; exit 1; fi"
	return "sh -c " + shellQuote(script)
}

func speedInstallFailure(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 500 {
		message = message[len(message)-500:]
	}
	if message == "" {
		return "请检查 SSH、sudo 权限和软件源；安装可能超时"
	}
	return message
}
