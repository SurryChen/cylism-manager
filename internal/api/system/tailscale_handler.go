package system

import (
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	security "github.com/cylism/cylism-manager/internal/api/shared/security"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	tailscaleservice "github.com/cylism/cylism-manager/internal/service/system"
	"github.com/gin-gonic/gin"
)

// TailscaleHandler Tailscale 管理 handler
type TailscaleHandler struct {
	configs repository.SystemConfigRepository
	encKey  []byte
	runtime tailscaleservice.Runtime
}

type tailscaleRuntime = tailscaleservice.Runtime

func NewTailscaleHandler(configs repository.SystemConfigRepository, encKey []byte) *TailscaleHandler {
	return &TailscaleHandler{configs: configs, encKey: encKey, runtime: tailscaleservice.HostRuntime{}}
}

func (h *TailscaleHandler) WithRuntime(runtime tailscaleRuntime) *TailscaleHandler {
	if runtime != nil {
		h.runtime = runtime
	}
	return h
}

// Init 本机 Tailscale 初始化 POST /api/tailscale/init
func (h *TailscaleHandler) Init(c *gin.Context) {
	var req struct {
		AuthKey string `json:"auth_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "缺少 auth_key 参数")
		return
	}

	// Validate auth key format: tskey-auth-<base64>
	if !strings.HasPrefix(req.AuthKey, "tskey-auth-") {
		apiShared.BadRequest(c, "Auth Key 格式无效")
		return
	}

	// Encrypt and store
	encVal, err := crypto.Encrypt(h.encKey, req.AuthKey)
	if err != nil {
		apiShared.InternalError(c, "加密 Auth Key 失败")
		return
	}
	if h.configs == nil {
		apiShared.InternalError(c, "存储 Auth Key 失败")
		return
	}
	if err := h.configs.SetSystemConfig("tailscale_auth_key", encVal); err != nil {
		apiShared.InternalError(c, "存储 Auth Key 失败")
		return
	}

	tsIP := ""

	// Install Tailscale if not present
	ctx := c.Request.Context()
	if !h.runtime.Installed(ctx) {
		out, err := h.runtime.Install(ctx)
		if err != nil {
			apiShared.Error(c, http.StatusInternalServerError, model.CodeInternalError,
				"安装 Tailscale 失败: "+string(out))
			return
		}
	}

	// tailscale up (args passed separately — no shell injection)
	out, err := h.runtime.Run(ctx, "tailscale", "up",
		"--auth-key="+req.AuthKey, "--hostname=cylism-control-plane", "--accept-routes")
	if err != nil {
		apiShared.Error(c, http.StatusInternalServerError, model.CodeInternalError,
			"注册 Tailscale 失败: "+string(out))
		return
	}

	// Get Tailscale IP
	ipOut, err := h.runtime.Run(ctx, "tailscale", "ip", "-4")
	if err == nil {
		tsIP = strings.TrimSpace(string(ipOut))
	}

	// Read k3s token from file (not via shell cat)
	k3sToken := ""
	tokenBytes, err := h.runtime.ReadFile(ctx, "/var/lib/rancher/k3s/server/node-token")
	if err == nil {
		k3sToken = strings.TrimSpace(string(tokenBytes))
		if tk, encErr := crypto.Encrypt(h.encKey, k3sToken); encErr == nil {
			_ = h.configs.SetSystemConfig("k3s_join_token", tk)
		}
	}

	model.Success(c, gin.H{
		"tailscale_ip": tsIP,
		"k3s_token":    k3sToken,
		"status":       "initialized",
	})
}

// Status 查询 Tailscale 状态 GET /api/tailscale/status
func (h *TailscaleHandler) Status(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.runtime.Installed(ctx) {
		model.Success(c, gin.H{"initialized": false, "ip": "", "online": false})
		return
	}

	ipOut, _ := h.runtime.Run(ctx, "tailscale", "ip", "-4")
	tsIP := strings.TrimSpace(string(ipOut))

	statusOut, _ := h.runtime.Run(ctx, "tailscale", "status")

	model.Success(c, gin.H{
		"initialized": true,
		"ip":          tsIP,
		"online":      tsIP != "",
		"status_raw":  string(statusOut),
	})
}

// InstallScript 返回脱敏的一键安装命令 GET /api/tailscale/install-script
func (h *TailscaleHandler) InstallScript(c *gin.Context) {
	if h.configs == nil {
		apiShared.NotFound(c, "Auth Key 未配置")
		return
	}
	encVal, err := h.configs.GetSystemConfig("tailscale_auth_key")
	if err != nil {
		apiShared.NotFound(c, "Auth Key 未配置")
		return
	}
	authKey, err := crypto.Decrypt(h.encKey, encVal)
	if err != nil {
		apiShared.InternalError(c, "解密 Auth Key 失败")
		return
	}
	// Only return a sanitized prefix — never expose the full key
	display := security.TokenDisplay(authKey, 8)
	model.Success(c, gin.H{
		"command": "tailscale up --auth-key=<your-key>",
		"prefix":  display,
	})
}
