package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"golang.org/x/crypto/ssh"
)

func TestSSHExecServerContextAuthenticatesWithStoredPassword(t *testing.T) {
	listener, passwordSeen := passwordSSHServer(t, "server-password")
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)
	encryptionKey := []byte("01234567890123456789012345678901")
	password, err := crypto.Encrypt(encryptionKey, "server-password")
	if err != nil {
		t.Fatal(err)
	}

	output, err := SSHExecServerContext(context.Background(), time.Second, &model.Server{
		Host:        "127.0.0.1",
		SSHPort:     address.Port,
		SSHUser:     "root",
		SSHAuthType: "password",
		SSHPassword: password,
	}, encryptionKey, "echo ok")
	if err != nil {
		t.Fatalf("SSHExecServerContext() error = %v", err)
	}
	if string(output) != "ok\n" {
		t.Fatalf("SSHExecServerContext() output = %q, want ok", output)
	}
	if !passwordSeen.Load() {
		t.Fatal("SSH password authentication was not attempted")
	}
}

func TestSSHExecServerOutputContextSeparatesStderr(t *testing.T) {
	listener, _ := passwordSSHServer(t, "server-password")
	defer listener.Close()
	key := []byte("01234567890123456789012345678901")
	password, err := crypto.Encrypt(key, "server-password")
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := SSHExecServerOutputContext(context.Background(), time.Second, &model.Server{
		Host: "127.0.0.1", SSHPort: listener.Addr().(*net.TCPAddr).Port,
		SSHUser: "root", SSHAuthType: "password", SSHPassword: password,
	}, key, "echo split")
	if err != nil || string(stdout) != "ok\n" || string(stderr) != "warning\n" {
		t.Fatalf("stdout=%q, stderr=%q, err=%v", stdout, stderr, err)
	}
}

func TestSSHExecServerContextIdentifiesAuthenticationFailure(t *testing.T) {
	listener, _ := passwordSSHServer(t, "server-password")
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)
	encryptionKey := []byte("01234567890123456789012345678901")
	password, err := crypto.Encrypt(encryptionKey, "wrong-password")
	if err != nil {
		t.Fatal(err)
	}
	_, err = SSHExecServerContext(context.Background(), time.Second, &model.Server{
		Host: "127.0.0.1", SSHPort: address.Port, SSHUser: "root",
		SSHAuthType: "password", SSHPassword: password,
	}, encryptionKey, "echo ok")
	var connectionError *SSHConnectionError
	if !errors.As(err, &connectionError) {
		t.Fatalf("expected SSHConnectionError, got %v", err)
	}
}

func TestProxySSHExecAndStreamingCommand(t *testing.T) {
	target, targetAuthenticated := passwordSSHServer(t, "target-password")
	defer target.Close()
	proxy, proxyAuthenticated := forwardingSSHServer(t, "proxy-password", target.Addr().String())
	defer proxy.Close()
	key := []byte("01234567890123456789012345678901")
	proxyPassword, _ := crypto.Encrypt(key, "proxy-password")
	targetPassword, _ := crypto.Encrypt(key, "target-password")
	proxyID := uint(1)
	server := &model.Server{
		ID: 2, Host: "127.0.0.1", SSHPort: target.Addr().(*net.TCPAddr).Port,
		SSHUser: "root", SSHAuthType: "password", SSHPassword: targetPassword,
		ProxyServerID: &proxyID,
		SSHProxy: &model.Server{ID: proxyID, Host: "127.0.0.1", SSHPort: proxy.Addr().(*net.TCPAddr).Port,
			SSHUser: "root", SSHAuthType: "password", SSHPassword: proxyPassword},
	}
	output, err := SSHExecServerContext(context.Background(), time.Second, server, key, "echo ok")
	if err != nil || string(output) != "ok\n" || !proxyAuthenticated.Load() || !targetAuthenticated.Load() {
		t.Fatalf("proxied command: output=%q, err=%v, proxy=%v, target=%v", output, err, proxyAuthenticated.Load(), targetAuthenticated.Load())
	}
	command, err := StartSSHCommandContext(context.Background(), time.Second, server, key, "echo ok")
	if err != nil {
		t.Fatal(err)
	}
	output, err = io.ReadAll(command.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil || string(output) != "ok\n" {
		t.Fatalf("streamed command: output=%q, err=%v", output, err)
	}
	client, cleanup, err := NewSSHClientContext(context.Background(), 500*time.Millisecond, server, key)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	time.Sleep(550 * time.Millisecond)
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("connection closed after dial timeout: %v", err)
	}
	output, err = session.CombinedOutput("echo ok")
	_ = session.Close()
	if err != nil || string(output) != "ok\n" {
		t.Fatalf("command after dial timeout: output=%q, err=%v", output, err)
	}
	server.SSHPassword, _ = crypto.Encrypt(key, "wrong-target-password")
	_, err = SSHExecServerContext(context.Background(), time.Second, server, key, "echo ok")
	var connectionError *SSHConnectionError
	if !errors.As(err, &connectionError) {
		t.Fatalf("target authentication must be a connection error: %v", err)
	}
}

func forwardingSSHServer(t *testing.T, password, allowedDestination string) (net.Listener, *atomic.Bool) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	authenticated := &atomic.Bool{}
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, input []byte) (*ssh.Permissions, error) {
		if string(input) != password {
			return nil, ssh.ErrNoAuth
		}
		authenticated.Store(true)
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					var destination struct {
						Host       string
						Port       uint32
						OriginHost string
						OriginPort uint32
					}
					if channel.ChannelType() != "direct-tcpip" || ssh.Unmarshal(channel.ExtraData(), &destination) != nil ||
						net.JoinHostPort(destination.Host, strconv.Itoa(int(destination.Port))) != allowedDestination {
						_ = channel.Reject(ssh.Prohibited, "destination denied")
						continue
					}
					upstream, err := net.Dial("tcp", allowedDestination)
					if err != nil {
						_ = channel.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					forward, requests, err := channel.Accept()
					if err != nil {
						upstream.Close()
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() {
						defer forward.Close()
						defer upstream.Close()
						go func() { _, _ = io.Copy(upstream, forward) }()
						_, _ = io.Copy(forward, upstream)
					}()
				}
			}()
		}
	}()
	return listener, authenticated
}

func passwordSSHServer(t *testing.T, expectedPassword string) (net.Listener, *atomic.Bool) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	passwordSeen := &atomic.Bool{}
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != expectedPassword {
			return nil, ssh.ErrNoAuth
		}
		passwordSeen.Store(true)
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(0)))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, channels, requests, handshakeErr := ssh.NewServerConn(connection, config)
				if handshakeErr != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "session" {
						_ = channel.Reject(ssh.UnknownChannelType, "session required")
						continue
					}
					client, requestStream, channelErr := channel.Accept()
					if channelErr != nil {
						return
					}
					go func() {
						defer client.Close()
						for request := range requestStream {
							if request.Type != "exec" {
								_ = request.Reply(false, nil)
								continue
							}
							_ = request.Reply(true, nil)
							var payload struct{ Command string }
							if ssh.Unmarshal(request.Payload, &payload) == nil && payload.Command == "echo split" {
								_, _ = client.Stderr().Write([]byte("warning\n"))
							}
							_, _ = client.Write([]byte("ok\n"))
							_, _ = client.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
							return
						}
					}()
				}
			}()
		}
	}()
	return listener, passwordSeen
}
