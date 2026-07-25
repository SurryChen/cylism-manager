package api

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type ServerHandler struct {
	store  *store.Store
	encKey []byte
}

func NewServerHandler(s *store.Store, encKey []byte) *ServerHandler {
	return &ServerHandler{store: s, encKey: encKey}
}

type createServerReq struct {
	Name             string `json:"name" binding:"required"`
	Host             string `json:"host" binding:"required"`
	SSHHost          string `json:"ssh_host"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHAuthType      string `json:"ssh_auth_type"`
	SSHPassword      string `json:"ssh_password"`
	SSHKey           string `json:"ssh_key"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase"`
}

func (h *ServerHandler) Create(c *gin.Context) {
	var req createServerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	if req.SSHPort == 0 { req.SSHPort = 22 }
	if req.SSHAuthType == "" { req.SSHAuthType = "password" }

	encPassword, _ := crypto.Encrypt(h.encKey, req.SSHPassword)
	encKey, _ := crypto.Encrypt(h.encKey, req.SSHKey)
	encPassphrase, _ := crypto.Encrypt(h.encKey, req.SSHKeyPassphrase)

	server := &model.Server{
		Name: req.Name, Host: req.Host,
		SSHHost: req.SSHHost, SSHPort: req.SSHPort, SSHUser: req.SSHUser,
		SSHAuthType: req.SSHAuthType, SSHPassword: encPassword,
		SSHKey: encKey, SSHKeyPassphrase: encPassphrase,
	}
	if req.SSHHost == "" { server.SSHHost = req.Host }

	if err := h.store.CreateServer(server); err != nil {
		model.Error(c, http.StatusConflict, model.CodeConflict, err.Error())
		return
	}
	model.Success(c, server)
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.Success(c, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found"); return }
	model.Success(c, server)
}

func (h *ServerHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil { model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found"); return }

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error()); return
	}
	if v, ok := updates["name"]; ok { server.Name = v.(string) }
	if v, ok := updates["ssh_password"]; ok { enc, _ := crypto.Encrypt(h.encKey, v.(string)); server.SSHPassword = enc }
	if v, ok := updates["ssh_key"]; ok { enc, _ := crypto.Encrypt(h.encKey, v.(string)); server.SSHKey = enc }
	if v, ok := updates["ssh_host"]; ok { server.SSHHost = v.(string) }
	if v, ok := updates["ssh_port"]; ok { server.SSHPort = int(v.(float64)) }
	if v, ok := updates["ssh_user"]; ok { server.SSHUser = v.(string) }
	if v, ok := updates["ssh_auth_type"]; ok { server.SSHAuthType = v.(string) }

	if err := h.store.UpdateServer(server); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error()); return
	}
	model.Success(c, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.store.DeleteServer(uint(id)); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error()); return
	}
	model.SuccessWithMessage(c, nil, "操作成功")
}


// Probe SSH 连通性检测 POST /api/servers/:id/probe
func (h *ServerHandler) Probe(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid id")
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}

	start := time.Now()
	reachable, errMsg := probeSSH(server, h.encKey)
	latency := time.Since(start).Milliseconds()

	if reachable {
		model.Success(c, gin.H{
			"reachable":   true,
			"latency_ms": latency,
		})
	} else {
		model.Success(c, gin.H{
			"reachable":   false,
			"error":      errMsg,
			"latency_ms": latency,
		})
	}
}

// Precheck 加入集群前置检测 POST /api/servers/:id/precheck
func (h *ServerHandler) Precheck(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid id")
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}

	checks := runPrechecks(server, h.encKey)
	allPass := true
	for _, ch := range checks {
		if !ch["pass"].(bool) {
			allPass = false
			break
		}
	}

	model.Success(c, gin.H{
		"checks":   checks,
		"all_pass": allPass,
	})
}


// --- SSH helper functions ---

// sshTimeout is the maximum duration for a single SSH command.
const sshTimeout = 15 * time.Second

func sshExec(timeout time.Duration, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return cmd.CombinedOutput()
}

// buildSSHArgs constructs SSH arguments. For key auth, writes a temp key file
// and schedules its removal via a one-shot goroutine.
func buildSSHArgs(server *model.Server, encKey []byte, host string) []string {
	port := server.SSHPort
	if port == 0 {
		port = 22
	}
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", port),
	}

	if server.SSHAuthType == "key" && server.SSHKey != "" {
		decKey, err := crypto.Decrypt(encKey, server.SSHKey)
		if err == nil {
			keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("cylism-ssh-%d-%d-%s", server.ID, time.Now().UnixNano(),
		func() string { b := make([]byte, 8); _, _ = rand.Read(b); return fmt.Sprintf("%x", b) }()))
			os.WriteFile(keyPath, []byte(decKey), 0600)
			args = append(args, "-i", keyPath)
			// Clean up after 60s
			go func() { time.Sleep(60 * time.Second); os.Remove(keyPath) }()
		}
	}
	// Password auth: sshpass is not reliably available. Require key auth for automation.

	args = append(args, fmt.Sprintf("%s@%s", server.SSHUser, host))
	return args
}

func probeSSH(server *model.Server, encKey []byte) (bool, string) {
	host := server.SSHHost
	if host == "" {
		host = server.Host
	}
	args := buildSSHArgs(server, encKey, host)
	args = append(args, "echo ok")
	out, err := sshExec(sshTimeout, args)
	if err != nil {
		return false, fmt.Sprintf("%s: %s", err.Error(), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)) == "ok", ""
}

func runPrechecks(server *model.Server, encKey []byte) []gin.H {
	host := server.SSHHost
	if host == "" {
		host = server.Host
	}
	args := buildSSHArgs(server, encKey, host)

	checks := make([]gin.H, 0, 5)

	// 1. SSH connect
	out, err := sshExec(sshTimeout, append(args, "echo ok"))
	sshOK := err == nil && strings.TrimSpace(string(out)) == "ok"
	checks = append(checks, gin.H{
		"name": "ssh_connect", "label": "SSH 连接",
		"pass": sshOK,
		"detail": func() string {
			if sshOK {
				return "连接成功"
			}
			return fmt.Sprintf("连接失败: %v", err)
		}(),
	})

	if !sshOK {
		for _, n := range []struct{ name, label string }{
			{"root_privilege", "Root 权限"},
			{"swap_disabled", "Swap 状态"},
			{"os_compatible", "操作系统"},
			{"disk_space", "磁盘空间"},
		} {
			checks = append(checks, gin.H{
				"name": n.name, "label": n.label,
				"pass": false, "detail": "SSH 不可达，跳过",
			})
		}
		return checks
	}

	// 2. root privilege
	out, err = sshExec(sshTimeout, append(args, "id -u"))
	rootOK := err == nil && strings.TrimSpace(string(out)) == "0"
	checks = append(checks, gin.H{
		"name": "root_privilege", "label": "Root 权限",
		"pass": rootOK, "detail": strings.TrimSpace(string(out)),
	})

	// 3. swap
	out, err = sshExec(sshTimeout, append(args, "swapon --show 2>/dev/null | wc -l"))
	swapOK := err == nil && strings.TrimSpace(string(out)) == "0"
	checks = append(checks, gin.H{
		"name": "swap_disabled", "label": "Swap 状态",
		"pass": swapOK,
		"detail": func() string {
			if swapOK {
				return "已关闭"
			}
			return "swap 已开启"
		}(),
	})

	// 4. OS
	out, err = sshExec(sshTimeout, append(args, "uname -m"))
	osOK := err == nil && (strings.Contains(string(out), "x86_64") || strings.Contains(string(out), "aarch64"))
	checks = append(checks, gin.H{
		"name": "os_compatible", "label": "操作系统",
		"pass": osOK, "detail": strings.TrimSpace(string(out)),
	})

	// 5. disk space
	out, err = sshExec(sshTimeout, append(args, "df -BG / | tail -1 | awk '{print $4}' | sed 's/G//'"))
	diskGB := 0
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &diskGB)
	}
	checks = append(checks, gin.H{
		"name": "disk_space", "label": "磁盘空间",
		"pass": diskGB >= 2, "detail": fmt.Sprintf("可用 %dGB", diskGB),
	})

	return checks
}
