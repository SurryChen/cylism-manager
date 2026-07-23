package deployer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"golang.org/x/crypto/ssh"
)

// AgentProbeResult Agent 状态探测结果
type AgentProbeResult struct {
	Installed      bool   `json:"installed"`
	BinaryExists   bool   `json:"binary_exists"`
	ProcessRunning bool   `json:"process_running"`
	SystemdExists  bool   `json:"systemd_exists"`
	SystemdActive  bool   `json:"systemd_active"`
	AgentVersion   string `json:"agent_version"`
	ListeningPort  int    `json:"listening_port"`
}

// SSHClient SSH 客户端封装
type SSHClient struct {
	server *model.Server
	client *ssh.Client
}

// NewSSHClient 根据服务器配置创建 SSH 客户端
// password 和 key 应为解密后的明文
func NewSSHClient(server *model.Server, password, key, passphrase string) (*SSHClient, error) {
	var authMethods []ssh.AuthMethod

	switch server.SSHAuthType {
	case "password":
		authMethods = append(authMethods, ssh.Password(password))
	case "key":
		var signer ssh.Signer
		var err error
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(key))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", server.SSHAuthType)
	}

	config := &ssh.ClientConfig{
		User:            server.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 一期：跳过 host key 校验，预留配置项
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(server.SSHHost, fmt.Sprintf("%d", server.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}

	return &SSHClient{server: server, client: client}, nil
}

// Close 关闭 SSH 连接
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// RunCmd 在远程服务器上执行命令，返回 stdout+stderr 合并输出
func (c *SSHClient) RunCmd(cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf

	if err := session.Run(cmd); err != nil {
		return buf.String(), fmt.Errorf("run: %w, output: %s", err, buf.String())
	}
	return buf.String(), nil
}

// DetectOSArch 探测远程服务器的操作系统和架构
func (c *SSHClient) DetectOSArch() (osName, arch string, err error) {
	output, err := c.RunCmd("uname -s && uname -m")
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) >= 2 {
		osName = strings.ToLower(lines[0]) // e.g., "linux"
		arch = strings.ToLower(lines[1])   // e.g., "x86_64", "aarch64"
	}
	if osName == "" {
		osName = "linux"
	}
	// 标准化 arch 名称
	switch arch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	}
	return osName, arch, nil
}

// UploadFile 通过 SFTP 上传文件到远程服务器
func (c *SSHClient) UploadFile(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	return c.WriteFile(remotePath, data, 0755)
}

// WriteFile 将内容写入远程文件
func (c *SSHClient) WriteFile(remotePath string, content []byte, mode os.FileMode) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	// 先创建目录
	dir := filepath.Dir(remotePath)
	_, _ = c.RunCmd(fmt.Sprintf("mkdir -p %s", dir))

	// 通过 stdin 管道写入文件
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.Copy(stdin, bytes.NewReader(content))
	}()

	return session.Run(fmt.Sprintf("cat > %s && chmod %o %s", remotePath, mode, remotePath))
}

// ProbeAgent 探测远端服务器的 Agent 状态
func (c *SSHClient) ProbeAgent() *AgentProbeResult {
	result := &AgentProbeResult{}

	// 检测二进制文件
	output, err := c.RunCmd("test -f /opt/cylism-manager/agent && echo 'exists' || echo 'missing'")
	if err == nil && strings.TrimSpace(output) == "exists" {
		result.BinaryExists = true
	}

	// 检测进程
	output, err = c.RunCmd("ps aux | grep 'cylism-agent' | grep -v grep | wc -l")
	if err == nil && strings.TrimSpace(output) != "0" {
		result.ProcessRunning = true
	}

	// 检测 systemd service 是否存在
	output, err = c.RunCmd("systemctl is-active cylism-agent 2>/dev/null || echo 'inactive'")
	if err == nil {
		status := strings.TrimSpace(output)
		result.SystemdExists = status != "inactive"
		result.SystemdActive = status == "active"
	}

	// 获取版本号
	if result.BinaryExists {
		output, err = c.RunCmd("/opt/cylism-manager/agent --version 2>/dev/null")
		if err == nil {
			result.AgentVersion = strings.TrimSpace(output)
		}
	}

	result.Installed = result.BinaryExists || result.ProcessRunning || result.SystemdExists
	return result
}

// DeployAgent 部署 Agent 到远程服务器
func (c *SSHClient) DeployAgent(agentBinPath string) error {
	remotePath := "/opt/cylism-manager/agent"
	logPath := "/opt/cylism-manager/agent.log"
	tlsOpts := "--tls-cert=/opt/cylism-manager/cert.pem --tls-key=/opt/cylism-manager/key.pem --tls-ca=/opt/cylism-manager/ca.pem"

	// 上传 Agent 二进制
	if err := c.UploadFile(agentBinPath, remotePath); err != nil {
		return fmt.Errorf("upload agent: %w", err)
	}

	// 通过 sudo tee 写入 systemd service 文件
	serviceContent := fmt.Sprintf(`[Unit]
Description=Cylism Manager Agent
After=network.target

[Service]
Type=simple
ExecStart=%s --port=%d %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`, remotePath, c.server.Port, tlsOpts)

	// 用 sudo tee 写入（SSH 用户有 sudo 免密）
	writeCmd := fmt.Sprintf("sudo tee /etc/systemd/system/cylism-agent.service > /dev/null << 'SYSTEMD_EOF'\n%s\nSYSTEMD_EOF", serviceContent)
	_, writeErr := c.RunCmd(writeCmd)
	if writeErr != nil {
		// systemd 写入失败，回退到 nohup 方式
		c.RunCmd("sudo pkill -f '/opt/cylism-manager/agent' || true")
		nohupCmd := fmt.Sprintf("nohup %s --port=%d %s > %s 2>&1 &", remotePath, c.server.Port, tlsOpts, logPath)
		_, err := c.RunCmd(nohupCmd)
		if err != nil {
			return fmt.Errorf("start agent (nohup): %w", err)
		}
		return nil
	}

	// 启用并启动 systemd service
	c.RunCmd("sudo systemctl daemon-reload")
	_, err := c.RunCmd("sudo systemctl enable --now cylism-agent")
	if err != nil {
		// systemd enable 失败，回退到 nohup
		c.RunCmd("sudo pkill -f '/opt/cylism-manager/agent' || true")
		nohupCmd := fmt.Sprintf("nohup %s --port=%d %s > %s 2>&1 &", remotePath, c.server.Port, tlsOpts, logPath)
		_, err = c.RunCmd(nohupCmd)
		if err != nil {
			return fmt.Errorf("start agent (fallback): %w", err)
		}
	}
	return nil
}
