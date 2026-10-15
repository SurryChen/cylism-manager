package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSSHDownloadProxyConnectStreamsWithoutHTTPHijack(t *testing.T) {
	client, proxyConnection := net.Pipe()
	defer client.Close()
	upstream, origin := net.Pipe()
	defer origin.Close()
	proxy := &sshDownloadProxy{
		dial: func(_ context.Context, address string) (net.Conn, error) {
			if address != "example.com:443" {
				t.Errorf("unexpected destination %q", address)
			}
			return upstream, nil
		},
	}
	done := make(chan struct{})
	go func() {
		proxy.serveConnection(noDeadlineConn{proxyConnection})
		close(done)
	}()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(client, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response := bufio.NewReader(client)
	if line, err := response.ReadString('\n'); err != nil || line != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("CONNECT response = %q, error = %v", line, err)
	}
	if line, err := response.ReadString('\n'); err != nil || line != "\r\n" {
		t.Fatalf("CONNECT header ending = %q, error = %v", line, err)
	}
	originDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(origin, buffer); err != nil {
			originDone <- err
			return
		}
		if string(buffer) != "ping" {
			originDone <- io.ErrUnexpectedEOF
			return
		}
		_, err := origin.Write([]byte("pong"))
		originDone <- err
	}()
	if _, err := io.WriteString(client, "ping"); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(response, buffer); err != nil || string(buffer) != "pong" {
		t.Fatalf("tunnel response = %q, error = %v", buffer, err)
	}
	if err := <-originDone; err != nil {
		t.Fatal(err)
	}
	_ = origin.Close()
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy connection did not close")
	}
}

type noDeadlineConn struct{ net.Conn }

func (noDeadlineConn) SetDeadline(time.Time) error      { return errors.ErrUnsupported }
func (noDeadlineConn) SetReadDeadline(time.Time) error  { return errors.ErrUnsupported }
func (noDeadlineConn) SetWriteDeadline(time.Time) error { return errors.ErrUnsupported }

func TestSSHDownloadProxyHTTPForward(t *testing.T) {
	client, proxyConnection := net.Pipe()
	defer client.Close()
	upstream, origin := net.Pipe()
	defer origin.Close()
	proxy := &sshDownloadProxy{dial: func(_ context.Context, address string) (net.Conn, error) {
		if address != "example.com:80" {
			t.Errorf("unexpected destination %q", address)
		}
		return upstream, nil
	}}
	go proxy.serveConnection(noDeadlineConn{proxyConnection})
	originDone := make(chan error, 1)
	go func() {
		request, err := http.ReadRequest(bufio.NewReader(origin))
		if err != nil {
			originDone <- err
			return
		}
		defer request.Body.Close()
		if request.Host != "example.com" || request.URL.Path != "/package" {
			originDone <- io.ErrUnexpectedEOF
			return
		}
		originDone <- (&http.Response{StatusCode: http.StatusOK, ProtoMajor: 1, ProtoMinor: 1,
			Body: io.NopCloser(strings.NewReader("ok")), ContentLength: 2}).Write(origin)
	}()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(client, "GET http://example.com/package HTTP/1.1\r\nHost: example.com\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("HTTP response = status %d body %q, error %v", response.StatusCode, body, err)
	}
	if err := <-originDone; err != nil {
		t.Fatal(err)
	}
}

func TestSSHOutboundCommandStreamsBothDirections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(connection, buffer); err != nil {
			serverDone <- err
			return
		}
		if string(buffer) != "ping" {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		_, err = connection.Write([]byte("pong"))
		serverDone <- err
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	command := exec.Command("sh", "-c", sshOutboundCommand(host, port))
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	marker, err := bufio.NewReader(stderr).ReadString('\n')
	if err != nil || strings.TrimSpace(marker) != "CYLISM_PROXY_READY "+host {
		t.Fatalf("outbound connection marker = %q, error = %v", marker, err)
	}
	if _, err := io.WriteString(stdin, "ping"); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	response := make([]byte, 4)
	if _, err := io.ReadFull(stdout, response); err != nil || string(response) != "pong" {
		t.Fatalf("outbound response = %q, error = %v", response, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestSSHOutboundCommandStreamsLargeResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	const responseSize = 32 << 20
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(15 * time.Second))
		request := make([]byte, 4)
		if _, err := io.ReadFull(connection, request); err != nil || !bytes.Equal(request, []byte("ping")) {
			serverDone <- err
			return
		}
		_, err = io.CopyN(connection, bytes.NewReader(bytes.Repeat([]byte("x"), responseSize)), responseSize)
		serverDone <- err
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	command := exec.Command("sh", "-c", sshOutboundCommand(host, port))
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	marker, err := bufio.NewReader(stderr).ReadString('\n')
	if err != nil || strings.TrimSpace(marker) != "CYLISM_PROXY_READY "+host {
		t.Fatalf("outbound connection marker = %q, error = %v", marker, err)
	}
	if _, err := io.WriteString(stdin, "ping"); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	received, err := io.Copy(io.Discard, stdout)
	if err != nil || received != responseSize {
		t.Fatalf("large response bytes = %d, error = %v", received, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}
