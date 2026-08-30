package system

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// TailscaleHandler Tailscale 管理 handler
type TailscaleHandler struct {
	store  *store.Store
	encKey []byte
}

func NewTailscaleHandler(s *store.Store, encKey []byte) *TailscaleHandler {
	return &TailscaleHandler{store: s, encKey: encKey}
}

// execWithTimeout runs a command with a timeout.
func execWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// sanitizeToken strips non-hex characters from a token string for safe display.
func sanitizeToken(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == ':' {
			return r
		}
		return -1
	}, s)
}

// Init 本机 Tailscale 初始化 POST /api/tailscale/init
func (h *TailscaleHandler) Init(c *gin.Context) {
	var req struct {
		AuthKey string `json:"auth_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "缺少 auth_key 参数")
		return
	}

	// Validate auth key format: tskey-auth-<base64>
	if !strings.HasPrefix(req.AuthKey, "tskey-auth-") {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "Auth Key 格式无效")
		return
	}

	// Encrypt and store
	encVal, err := crypto.Encrypt(h.encKey, req.AuthKey)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "加密 Auth Key 失败")
		return
	}
	if err := h.store.SetSystemConfig("tailscale_auth_key", encVal); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "存储 Auth Key 失败")
		return
	}

	tsIP := ""

	// Install Tailscale if not present
	if _, err := exec.LookPath("tailscale"); err != nil {
		out, err := execWithTimeout(120*time.Second, "sh", "-c",
			"curl -fsSL https://tailscale.com/install.sh | sh")
		if err != nil {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError,
				"安装 Tailscale 失败: "+string(out))
			return
		}
	}

	// tailscale up (args passed separately — no shell injection)
	out, err := execWithTimeout(60*time.Second, "tailscale", "up",
		"--auth-key="+req.AuthKey, "--hostname=cylism-control-plane", "--accept-routes")
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError,
			"注册 Tailscale 失败: "+string(out))
		return
	}

	// Get Tailscale IP
	ipOut, err := execWithTimeout(10*time.Second, "tailscale", "ip", "-4")
	if err == nil {
		tsIP = strings.TrimSpace(string(ipOut))
	}

	// Read k3s token from file (not via shell cat)
	k3sToken := ""
	tokenBytes, err := os.ReadFile("/var/lib/rancher/k3s/server/node-token")
	if err == nil {
		k3sToken = strings.TrimSpace(string(tokenBytes))
		if tk, encErr := crypto.Encrypt(h.encKey, k3sToken); encErr == nil {
			h.store.SetSystemConfig("k3s_join_token", tk)
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
	if _, err := exec.LookPath("tailscale"); err != nil {
		model.Success(c, gin.H{"initialized": false, "ip": "", "online": false})
		return
	}

	ipOut, _ := execWithTimeout(5*time.Second, "tailscale", "ip", "-4")
	tsIP := strings.TrimSpace(string(ipOut))

	statusOut, _ := execWithTimeout(5*time.Second, "tailscale", "status")

	model.Success(c, gin.H{
		"initialized": true,
		"ip":          tsIP,
		"online":      tsIP != "",
		"status_raw":  string(statusOut),
	})
}

// InstallScript 返回脱敏的一键安装命令 GET /api/tailscale/install-script
func (h *TailscaleHandler) InstallScript(c *gin.Context) {
	encVal, err := h.store.GetSystemConfig("tailscale_auth_key")
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "Auth Key 未配置")
		return
	}
	authKey, err := crypto.Decrypt(h.encKey, encVal)
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "解密 Auth Key 失败")
		return
	}
	// Only return a sanitized prefix — never expose the full key
	display := sanitizeToken(authKey)
	if len(display) > 20 {
		display = display[:8] + "..." + display[len(display)-8:]
	}
	model.Success(c, gin.H{
		"command": "tailscale up --auth-key=<your-key>",
		"prefix":  display,
	})
}
