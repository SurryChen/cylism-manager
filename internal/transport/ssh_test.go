package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
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
					_, _ = client.Write([]byte("ok\n"))
					_, _ = client.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					return
				}
			}()
		}
	}()
	return listener, passwordSeen
}
