package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"golang.org/x/crypto/ssh"
)

// SSHTimeout is the default bound for a single remote SSH command.
const SSHTimeout = 15 * time.Second

// SSHConnectionError identifies failures before the remote command starts.
type SSHConnectionError struct{ Err error }

func (e *SSHConnectionError) Error() string { return e.Err.Error() }
func (e *SSHConnectionError) Unwrap() error { return e.Err }

// SSHExecServerContext executes a remote command using the credential stored
// for server. Credentials are passed directly to the Go SSH client, never to
// a process argument, environment variable, or temporary key file.
func SSHExecServerContext(parent context.Context, timeout time.Duration, server *model.Server, encKey []byte, command string) ([]byte, error) {
	client, cleanup, err := newSSHClientContext(parent, timeout, server, encKey)
	if err != nil {
		return nil, &SSHConnectionError{Err: err}
	}
	defer cleanup()

	session, err := client.NewSession()
	if err != nil {
		return nil, &SSHConnectionError{Err: err}
	}
	defer session.Close()
	return session.CombinedOutput(command)
}

// SSHExecServerOutputContext keeps stdout separate when a command returns structured data.
func SSHExecServerOutputContext(parent context.Context, timeout time.Duration, server *model.Server, encKey []byte, command string) ([]byte, []byte, error) {
	client, cleanup, err := newSSHClientContext(parent, timeout, server, encKey)
	if err != nil {
		return nil, nil, &SSHConnectionError{Err: err}
	}
	defer cleanup()

	session, err := client.NewSession()
	if err != nil {
		return nil, nil, &SSHConnectionError{Err: err}
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(command)
	return stdout.Bytes(), stderr.Bytes(), err
}

// SSHCommand is a started remote command with streaming standard I/O. It is
// used for large transfers where buffering all output in memory is unsafe.
type SSHCommand struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader

	session *ssh.Session
	cleanup func()
	once    sync.Once
}

// StartSSHCommandContext starts a remote command and exposes its streams.
// Call Wait after copying the streams, or Close to stop the remote command.
func StartSSHCommandContext(parent context.Context, timeout time.Duration, server *model.Server, encKey []byte, command string) (*SSHCommand, error) {
	client, cleanup, err := newSSHClientContext(parent, timeout, server, encKey)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		cleanup()
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		cleanup()
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		cleanup()
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		cleanup()
		return nil, err
	}
	if err := session.Start(command); err != nil {
		session.Close()
		cleanup()
		return nil, err
	}
	return &SSHCommand{Stdin: stdin, Stdout: stdout, Stderr: stderr, session: session, cleanup: cleanup}, nil
}

// Wait waits for the remote command and releases its SSH connection.
func (c *SSHCommand) Wait() error {
	if c == nil || c.session == nil {
		return fmt.Errorf("SSH command is not initialized")
	}
	err := c.session.Wait()
	c.Close()
	return err
}

// Close stops the remote command and closes the associated SSH client.
func (c *SSHCommand) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.session != nil {
			_ = c.session.Close()
		}
		if c.cleanup != nil {
			c.cleanup()
		}
	})
}

func newSSHClientContext(parent context.Context, timeout time.Duration, server *model.Server, encKey []byte) (*ssh.Client, func(), error) {
	if parent == nil {
		return nil, nil, fmt.Errorf("ssh context is required")
	}
	if server == nil {
		return nil, nil, fmt.Errorf("ssh server is required")
	}
	if strings.TrimSpace(server.Host) == "" {
		return nil, nil, fmt.Errorf("ssh host is required")
	}
	config, err := sshClientConfig(server, encKey)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	port := server.SSHPort
	if port <= 0 {
		port = 22
	}
	address := net.JoinHostPort(server.Host, strconv.Itoa(port))
	var proxyClient *ssh.Client
	var proxyCleanup func()
	if server.ProxyServerID != nil {
		if server.SSHProxy == nil || server.SSHProxy.ID != *server.ProxyServerID || server.SSHProxy.ProxyServerID != nil || server.SSHProxy.ID == server.ID {
			return nil, nil, fmt.Errorf("SSH 代理节点不可用或不支持多级代理")
		}
		proxyClient, proxyCleanup, err = newSSHClientContext(parent, timeout, server.SSHProxy, encKey)
		if err != nil {
			return nil, nil, fmt.Errorf("连接 SSH 代理节点失败: %w", err)
		}
	}
	var connection net.Conn
	if proxyClient != nil {
		stopDial := context.AfterFunc(ctx, func() { _ = proxyClient.Close() })
		connection, err = proxyClient.Dial("tcp", address)
		stopDial()
	} else {
		connection, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		if proxyCleanup != nil {
			proxyCleanup()
		}
		return nil, nil, fmt.Errorf("连接目标 %s 失败: %w", address, err)
	}
	stopHandshake := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopHandshake()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		_ = connection.Close()
		if proxyCleanup != nil {
			proxyCleanup()
		}
		return nil, nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	client := ssh.NewClient(clientConnection, channels, requests)
	stop := make(chan struct{})
	go func() {
		select {
		case <-parent.Done():
			_ = client.Close()
		case <-stop:
		}
	}()
	cleanup := func() {
		close(stop)
		_ = client.Close()
		if proxyCleanup != nil {
			proxyCleanup()
		}
	}
	return client, cleanup, nil
}

// NewSSHClientContext is shared by command, transfer, and terminal sessions.
func NewSSHClientContext(parent context.Context, timeout time.Duration, server *model.Server, encKey []byte) (*ssh.Client, func(), error) {
	return newSSHClientContext(parent, timeout, server, encKey)
}

func sshClientConfig(server *model.Server, encKey []byte) (*ssh.ClientConfig, error) {
	auth, err := sshAuthMethod(server, encKey)
	if err != nil {
		return nil, err
	}
	user := strings.TrimSpace(server.SSHUser)
	if user == "" {
		user = "root"
	}
	return &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Matches the existing managed-host SSH behavior.
		Timeout:         SSHTimeout,
	}, nil
}

func sshAuthMethod(server *model.Server, encKey []byte) (ssh.AuthMethod, error) {
	switch server.SSHAuthType {
	case "password", "":
		if server.SSHPassword == "" {
			return nil, fmt.Errorf("ssh password is not configured")
		}
		password, err := crypto.Decrypt(encKey, server.SSHPassword)
		if err != nil {
			return nil, fmt.Errorf("decrypt ssh password: %w", err)
		}
		if password == "" {
			return nil, fmt.Errorf("ssh password is not configured")
		}
		return ssh.Password(password), nil
	case "key":
		if server.SSHKey == "" {
			return nil, fmt.Errorf("ssh private key is not configured")
		}
		privateKey, err := crypto.Decrypt(encKey, server.SSHKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt ssh private key: %w", err)
		}
		signer, err := parseSSHPrivateKey(privateKey, server.SSHKeyPassphrase, encKey)
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported ssh authentication type %q", server.SSHAuthType)
	}
}

func parseSSHPrivateKey(privateKey, encryptedPassphrase string, encKey []byte) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err == nil {
		return signer, nil
	}
	if _, needsPassphrase := err.(*ssh.PassphraseMissingError); !needsPassphrase {
		return nil, fmt.Errorf("parse ssh private key: %w", err)
	}
	if encryptedPassphrase == "" {
		return nil, fmt.Errorf("ssh private key passphrase is not configured")
	}
	passphrase, decryptErr := crypto.Decrypt(encKey, encryptedPassphrase)
	if decryptErr != nil {
		return nil, fmt.Errorf("decrypt ssh private key passphrase: %w", decryptErr)
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse ssh private key with passphrase: %w", err)
	}
	return signer, nil
}
