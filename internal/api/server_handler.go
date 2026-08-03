package api

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

type ServerHandler struct {
	store          *store.Store
	encKey         []byte
	statsCollector func(*model.Server) (gin.H, error)
}

func NewServerHandler(s *store.Store, encKey []byte) *ServerHandler {
	h := &ServerHandler{store: s, encKey: encKey}
	h.statsCollector = h.collectResourceStats
	return h
}

type createServerReq struct {
	Name             string `json:"name" binding:"required"`
	Host             string `json:"host" binding:"required"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHAuthType      string `json:"ssh_auth_type"`
	SSHPassword      string `json:"ssh_password"`
	SSHKey           string `json:"ssh_key"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase"`
}

func (h *ServerHandler) Create(c *gin.Context) {
	var req createServerReq
	log.Printf("[CreateServer] request: name=%q host=%q", req.Name, req.Host)
	if err := c.ShouldBindJSON(&req); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHAuthType == "" {
		req.SSHAuthType = "password"
	}

	sshKeyHash := ""
	if req.SSHKey != "" {
		h := md5.Sum([]byte(req.SSHKey))
		sshKeyHash = hex.EncodeToString(h[:])
	}
	encPassword, _ := crypto.Encrypt(h.encKey, req.SSHPassword)
	encKey, _ := crypto.Encrypt(h.encKey, req.SSHKey)
	encPassphrase, _ := crypto.Encrypt(h.encKey, req.SSHKeyPassphrase)

	server := &model.Server{
		Name: req.Name, Host: req.Host,
		SSHPort: req.SSHPort, SSHUser: req.SSHUser,
		SSHAuthType: req.SSHAuthType, SSHPassword: encPassword,
		SSHKey: encKey, SSHKeyPassphrase: encPassphrase,
		SSHKeyHash: sshKeyHash,
	}

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
		log.Printf("[ListServers] returned %d servers", len(servers))
		return
	}
	if K8s != nil {
		nodes, err := K8s.ListNodeInfos()
		if err != nil {
			// Preserve bindings when the cluster cannot be queried; a transient API
			// outage must not make registered servers appear to have left the cluster.
			log.Printf("[ListServers] skip cluster binding reconciliation: %v", err)
		} else {
			existingNodes := make(map[string]struct{}, len(nodes))
			for _, node := range nodes {
				existingNodes[node.Name] = struct{}{}
			}
			for index := range servers {
				server := &servers[index]
				if server.ClusterRole == "" && server.K8sNodeName == "" {
					continue
				}
				if server.K8sNodeName != "" {
					if _, exists := existingNodes[server.K8sNodeName]; exists {
						continue
					}
				}
				if server.K8sNodeName == "" {
					server.ClusterRole = ""
					if err := h.store.UpdateServer(server); err != nil {
						log.Printf("[ListServers] clear incomplete cluster binding on server %d failed: %v", server.ID, err)
					}
					continue
				}
				if err := h.store.UnbindServersFromClusterNode(server.K8sNodeName); err != nil {
					log.Printf("[ListServers] unbind server %d from removed node %q failed: %v", server.ID, server.K8sNodeName, err)
					continue
				}
				server.ClusterRole = ""
				server.K8sNodeName = ""
			}
		}
	}
	model.Success(c, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	model.Success(c, server)
}

func (h *ServerHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	log.Printf("[UpdateServer] id=%d, old host=%q", id, server.Host)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	log.Printf("[UpdateServer] request body: %+v", updates)
	if v, ok := updates["name"]; ok {
		server.Name = v.(string)
	}
	if v, ok := updates["host"]; ok {
		server.Host = v.(string)
	}
	if v, ok := updates["ssh_password"]; ok {
		enc, _ := crypto.Encrypt(h.encKey, v.(string))
		server.SSHPassword = enc
	}
	if v, ok := updates["ssh_key"]; ok {
		keyStr := v.(string)
		enc, _ := crypto.Encrypt(h.encKey, keyStr)
		server.SSHKey = enc
		if keyStr != "" {
			hh := md5.Sum([]byte(keyStr))
			server.SSHKeyHash = hex.EncodeToString(hh[:])
		} else {
			server.SSHKeyHash = ""
		}
	}
	if v, ok := updates["ssh_port"]; ok {
		server.SSHPort = int(v.(float64))
	}
	if v, ok := updates["ssh_user"]; ok {
		server.SSHUser = v.(string)
	}
	if v, ok := updates["ssh_auth_type"]; ok {
		server.SSHAuthType = v.(string)
	}

	if err := h.store.UpdateServer(server); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	log.Printf("[UpdateServer] after update: %+v", server)
	model.Success(c, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.store.DeleteServer(uint(id)); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, nil, "操作成功")
}

// Unbind removes only the platform's association between a server and a
// Kubernetes Node. It deliberately does not modify the Node or its workloads.
func (h *ServerHandler) Unbind(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "invalid server id")
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "server not found")
		return
	}
	server.ClusterRole = ""
	server.K8sNodeName = ""
	if err := h.store.UpdateServer(server); err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	model.SuccessWithMessage(c, server, "已解除集群绑定")
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
			"reachable":  true,
			"latency_ms": latency,
		})
	} else {
		model.Success(c, gin.H{
			"reachable":  false,
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
	log.Printf("[sshExec] running: ssh %v", args)
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
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", port),
	}

	if server.SSHAuthType == "key" && server.SSHKey != "" {
		decKey, err := crypto.Decrypt(encKey, server.SSHKey)
		if err == nil {
			log.Printf("[buildSSHArgs] key decrypt success for server=%d, user=%s, host=%s", server.ID, server.SSHUser, host)
			// 校验解密后的密钥内容是否完整（对比 MD5）
			if server.SSHKeyHash != "" {
				got := md5.Sum([]byte(decKey))
				gotHex := hex.EncodeToString(got[:])
				if gotHex != server.SSHKeyHash {
					log.Printf("[buildSSHArgs] KEY HASH MISMATCH for server=%d: expected=%s got=%s (decKey len=%d, first 32 bytes=%q)",
						server.ID, server.SSHKeyHash, gotHex, len(decKey), safePrefix(decKey, 32))
				} else {
					log.Printf("[buildSSHArgs] key hash verified OK for server=%d", server.ID)
				}
			}
			// 写入固定路径（/data/tmp/ 持久卷可写，避免 Alpine /tmp 问题）
			keyDir := filepath.Join("/data", "tmp")
			os.MkdirAll(keyDir, 0700)
			keyPath := filepath.Join(keyDir, fmt.Sprintf("cylism-ssh-%d", server.ID))
			// 确保 PEM 密钥以换行符结尾（缺少会导致 libcrypto 错误）
			keyContent := decKey
			if len(keyContent) > 0 && keyContent[len(keyContent)-1] != '\n' {
				keyContent = keyContent + "\n"
				log.Printf("[buildSSHArgs] added trailing newline to key for server=%d", server.ID)
			}
			if err := os.WriteFile(keyPath, []byte(keyContent), 0600); err != nil {
				log.Printf("[buildSSHArgs] write key file FAILED: %v", err)
			} else {
				if fi, statErr := os.Stat(keyPath); statErr == nil {
					log.Printf("[buildSSHArgs] key file written: size=%d", fi.Size())
				}
				// 打印密钥文件首尾字节，用于诊断截断/损坏
				if len(decKey) > 0 {
					log.Printf("[buildSSHArgs] key HEAD: %q", safePrefix(decKey, 64))
					tailStart := len(decKey) - 80
					if tailStart < 0 {
						tailStart = 0
					}
					log.Printf("[buildSSHArgs] key TAIL: %q", decKey[tailStart:])
				}
			}
			args = append(args, "-i", keyPath)
		} else {
			log.Printf("[buildSSHArgs] key decrypt FAILED for server=%d, user=%s, host=%s: %v", server.ID, server.SSHUser, host, err)
		}
	}
	// Password auth: sshpass is not reliably available. Require key auth for automation.

	args = append(args, fmt.Sprintf("%s@%s", server.SSHUser, host))
	return args
}

// safePrefix 返回字符串前 n 个字符，避免日志泄露完整密钥
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func probeSSH(server *model.Server, encKey []byte) (bool, string) {
	host := server.Host
	args := buildSSHArgs(server, encKey, host)
	args = append(args, "echo ok")
	out, err := sshExec(sshTimeout, args)
	if err != nil {
		errMsg := fmt.Sprintf("%s: %s", err.Error(), strings.TrimSpace(string(out)))
		log.Printf("[probe] SSH probe failed (server=%s): %s", host, errMsg)
		return false, errMsg
	}
	// 使用 Contains 兼容 CombinedOutput 混入 stderr 警告（如 known_hosts 无法写入）
	return strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok", ""
}

func runPrechecks(server *model.Server, encKey []byte) []gin.H {
	host := server.Host
	args := buildSSHArgs(server, encKey, host)

	checks := make([]gin.H, 0, 5)

	// 1. SSH connect
	out, err := sshExec(sshTimeout, append(args, "echo ok"))
	sshOK := err == nil && (strings.Contains(string(out), "\nok") || strings.TrimSpace(string(out)) == "ok")
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

	// 2. root privilege: check uid==0 first, then sudo capability
	out, err = sshExec(sshTimeout, append(args, "id -u"))
	rootOK := err == nil && strings.TrimSpace(string(out)) == "0"
	rootDetail := strings.TrimSpace(string(out))
	if !rootOK {
		// Not root user; check sudo permission
		sudoOut, sudoErr := sshExec(sshTimeout, append(args, "sudo -n true 2>&1"))
		if sudoErr == nil || strings.Contains(string(sudoOut), "password") {
			rootOK = true
			rootDetail = "有 sudo 权限（非 root 用户）"
		} else {
			rootDetail = fmt.Sprintf("无 root 权限且无 sudo: %s", strings.TrimSpace(string(sudoOut)))
		}
	}
	checks = append(checks, gin.H{
		"name": "root_privilege", "label": "Root 权限",
		"pass": rootOK, "detail": rootDetail,
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

// Stats 服务器资源使用情况 GET /api/servers/:id/stats
func (h *ServerHandler) Stats(c *gin.Context) {
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

	host := server.Host
	args := buildSSHArgs(server, h.encKey, host)

	result := gin.H{}

	// 一个 SSH 调用拿所有数据（避免多次连接开销）
	out, err := sshExec(sshTimeout, append(args,
		"echo 'CPU:' $(top -bn1 | awk '/^%Cpu|^CPU:/{print 100-$8}');"+
			"echo 'MEM:' $(free -m | awk '/^Mem:/{print $2,$3,$7}');"+
			"echo 'DISK:' $(df -BG / | awk 'NR==2{print $2,$3,$4,$5}' | sed 's/G//g');"+
			"echo 'LOAD:' $(cat /proc/loadavg | awk '{print $1,$2,$3}');"+
			"echo 'UP:' $(uptime -p | sed 's/up //')"))

	if err == nil {
		raw := string(out)
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "CPU:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					cpuVal := 0.0
					fmt.Sscanf(parts[1], "%f", &cpuVal)
					result["cpu_percent"] = cpuVal
				}
			} else if strings.HasPrefix(line, "MEM:") {
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					total, _ := strconv.Atoi(parts[1])
					used, _ := strconv.Atoi(parts[2])
					avail, _ := strconv.Atoi(parts[3])
					result["memory_total_mb"] = total
					result["memory_used_mb"] = used
					result["memory_available_mb"] = avail
				}
			} else if strings.HasPrefix(line, "DISK:") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					dTotal, _ := strconv.Atoi(parts[1])
					dUsed, _ := strconv.Atoi(parts[2])
					dAvail, _ := strconv.Atoi(parts[3])
					result["disk_total_gb"] = dTotal
					result["disk_used_gb"] = dUsed
					result["disk_available_gb"] = dAvail
					if len(parts) >= 4 {
						result["disk_percent"] = parts[4]
					}
				}
			} else if strings.HasPrefix(line, "LOAD:") {
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					l1, _ := strconv.ParseFloat(parts[1], 64)
					l5, _ := strconv.ParseFloat(parts[2], 64)
					l15, _ := strconv.ParseFloat(parts[3], 64)
					result["load_1m"] = l1
					result["load_5m"] = l5
					result["load_15m"] = l15
				}
			} else if strings.HasPrefix(line, "UP:") {
				result["uptime"] = strings.TrimPrefix(line, "UP: ")
			}
		}
	}

	model.Success(c, result)
}

// ResourceStats samples every registered server with bounded concurrency. It
// is used by the server overview so opening the monitoring view cannot create
// one browser request and one SSH handshake per table row.
func (h *ServerHandler) ResourceStats(c *gin.Context) {
	servers, err := h.store.ListServers()
	if err != nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, err.Error())
		return
	}
	results := make([]gin.H, len(servers))
	semaphore := make(chan struct{}, 3)
	var group sync.WaitGroup
	for index := range servers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			started := time.Now()
			stats, statsErr := h.statsCollector(&servers[index])
			if stats == nil {
				stats = gin.H{}
			}
			stats["server_id"] = servers[index].ID
			stats["server_name"] = servers[index].Name
			stats["sampled_at"] = started.UTC().Format(time.RFC3339)
			stats["duration_ms"] = time.Since(started).Milliseconds()
			if statsErr != nil {
				stats["status"] = "unreachable"
				stats["error"] = statsErr.Error()
			} else if stats["status"] == nil {
				stats["status"] = "ready"
			}
			results[index] = stats
		}(index)
	}
	group.Wait()
	model.Success(c, results)
}

func (h *ServerHandler) collectResourceStats(server *model.Server) (gin.H, error) {
	host := server.Host
	args := buildSSHArgs(server, h.encKey, host)
	out, err := sshExec(5*time.Second, append(args,
		"echo 'CPU:' $(top -bn1 | awk '/^%Cpu|^CPU:/{print 100-$8}');"+
			"echo 'CPU_CORES:' $(nproc);"+
			"echo 'MEM:' $(free -m | awk '/^Mem:/{print $2,$3,$7}');"+
			"echo 'DISK:' $(df -BG / | awk 'NR==2{print $2,$3,$4,$5}' | sed 's/G//g');"+
			"echo 'LOAD:' $(cat /proc/loadavg | awk '{print $1,$2,$3}');"+
			"echo 'UP:' $(uptime -p | sed 's/up //')"))
	if err != nil {
		return gin.H{}, err
	}
	return parseServerStats(string(out)), nil
}

func parseServerStats(raw string) gin.H {
	result := gin.H{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CPU:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cpuVal := 0.0
				fmt.Sscanf(parts[1], "%f", &cpuVal)
				result["cpu_percent"] = cpuVal
			}
		} else if strings.HasPrefix(line, "CPU_CORES:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cores, _ := strconv.Atoi(parts[1])
				result["cpu_cores"] = cores
			}
		} else if strings.HasPrefix(line, "MEM:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				total, _ := strconv.Atoi(parts[1])
				used, _ := strconv.Atoi(parts[2])
				avail, _ := strconv.Atoi(parts[3])
				result["memory_total_mb"] = total
				result["memory_used_mb"] = used
				result["memory_available_mb"] = avail
			}
		} else if strings.HasPrefix(line, "DISK:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				dTotal, _ := strconv.Atoi(parts[1])
				dUsed, _ := strconv.Atoi(parts[2])
				dAvail, _ := strconv.Atoi(parts[3])
				result["disk_total_gb"] = dTotal
				result["disk_used_gb"] = dUsed
				result["disk_available_gb"] = dAvail
				if len(parts) >= 4 {
					result["disk_percent"] = parts[4]
				}
			}
		} else if strings.HasPrefix(line, "LOAD:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				l1, _ := strconv.ParseFloat(parts[1], 64)
				l5, _ := strconv.ParseFloat(parts[2], 64)
				l15, _ := strconv.ParseFloat(parts[3], 64)
				result["load_1m"] = l1
				result["load_5m"] = l5
				result["load_15m"] = l15
			}
		} else if strings.HasPrefix(line, "UP:") {
			result["uptime"] = strings.TrimPrefix(line, "UP: ")
		}
	}
	return result
}

// Terminal WebSocket 在线终端 GET /api/servers/:id/terminal
func (h *ServerHandler) Terminal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		return
	}

	conn, err := wsUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	defer conn.Close()

	// 构建 SSH 客户端配置
	port := server.SSHPort
	if port == 0 {
		port = 22
	}
	sshConfig := &ssh.ClientConfig{
		User:            server.SSHUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if server.SSHAuthType == "key" && server.SSHKey != "" {
		decKey, err := crypto.Decrypt(h.encKey, server.SSHKey)
		if err == nil {
			signer, signErr := ssh.ParsePrivateKey([]byte(decKey))
			if signErr == nil {
				sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
			} else {
				log.Printf("[terminal] key parse failed for server=%d: %v", server.ID, signErr)
				return
			}
		} else {
			log.Printf("[terminal] key decrypt failed for server=%d: %v", server.ID, err)
			return
		}
	} else if server.SSHAuthType == "password" && server.SSHPassword != "" {
		decPwd, err := crypto.Decrypt(h.encKey, server.SSHPassword)
		if err == nil {
			sshConfig.Auth = []ssh.AuthMethod{ssh.Password(decPwd)}
		} else {
			log.Printf("[terminal] password decrypt failed for server=%d: %v", server.ID, err)
			return
		}
	} else {
		log.Printf("[terminal] no auth method for server=%d", server.ID)
		return
	}

	addr := fmt.Sprintf("%s:%d", server.Host, port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		log.Printf("[terminal] dial failed for %s: %v", addr, err)
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Printf("[terminal] session failed: %v", err)
		return
	}
	defer session.Close()

	// 申请 PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 40, 80, modes); err != nil {
		log.Printf("[terminal] pty failed: %v", err)
		return
	}

	sessionIn, _ := session.StdinPipe()
	sessionOut, _ := session.StdoutPipe()

	if err := session.Shell(); err != nil {
		log.Printf("[terminal] shell failed: %v", err)
		return
	}

	// WebSocket → SSH stdin（支持 resize 和普通输入）
	go func() {
		for {
			data, err := conn.ReadFrame()
			if err != nil {
				log.Printf("[terminal] websocket input closed for server=%d: %v", server.ID, err)
				session.Close()
				return
			}
			if len(data) == 0 {
				continue
			}
			// 尝试解析 JSON resize 消息
			var resizeMsg struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(data, &resizeMsg) == nil && resizeMsg.Type == "resize" && resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
				session.WindowChange(resizeMsg.Rows, resizeMsg.Cols)
				continue
			}
			if _, err := sessionIn.Write(data); err != nil {
				log.Printf("[terminal] SSH input write failed for server=%d: %v", server.ID, err)
				session.Close()
				return
			}
		}
	}()

	// SSH stdout → WebSocket
	buf := make([]byte, 4096)
	for {
		n, err := sessionOut.Read(buf)
		if err != nil {
			break
		}
		if n > 0 {
			if err := conn.WriteFrame(buf[:n]); err != nil {
				log.Printf("[terminal] websocket output failed for server=%d: %v", server.ID, err)
				break
			}
		}
	}

	session.Close()
}
