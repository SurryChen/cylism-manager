package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"golang.org/x/crypto/ssh"
)

// StartSSHDownloadProxyContext makes a temporary proxy available on the target's
// loopback interface. Outbound connections are opened from the selected server.
type SSHDownloadProxyTrace struct {
	mu    sync.Mutex
	stage string
}

func (t *SSHDownloadProxyTrace) record(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.stage = stage
	t.mu.Unlock()
	log.Printf("[sing-box-install] %s", stage)
}

func (t *SSHDownloadProxyTrace) Stage() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stage
}

func StartSSHDownloadProxyContext(ctx context.Context, target, egress *model.Server, key []byte, trace *SSHDownloadProxyTrace) (string, func(), error) {
	trace.record(fmt.Sprintf("target=%s egress=%s connecting egress SSH", target.Host, egress.Host))
	egressClient, closeEgress, err := newSSHClientContext(ctx, 15*time.Second, egress, key)
	if err != nil {
		return "", nil, fmt.Errorf("连接下载出口节点失败: %w", err)
	}
	trace.record(fmt.Sprintf("target=%s egress=%s connecting target SSH", target.Host, egress.Host))
	targetClient, closeTarget, err := newSSHClientContext(ctx, 15*time.Second, target, key)
	if err != nil {
		closeEgress()
		return "", nil, fmt.Errorf("连接目标服务器失败: %w", err)
	}
	listener, err := targetClient.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		closeTarget()
		closeEgress()
		return "", nil, fmt.Errorf("目标服务器不允许 SSH 端口转发: %w", err)
	}
	handler := &sshDownloadProxy{
		dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialSSHOutbound(ctx, egressClient, address)
		},
		egressHost: egress.Host,
		trace:      trace,
	}
	var once sync.Once
	closed := make(chan struct{})
	closeProxy := func() {
		once.Do(func() {
			_ = listener.Close()
			closeTarget()
			closeEgress()
			close(closed)
		})
	}
	go handler.serve(listener)
	trace.record(fmt.Sprintf("target=%s egress=%s listen=%s waiting for CONNECT", target.Host, egress.Host, listener.Addr()))
	go func() {
		select {
		case <-ctx.Done():
			closeProxy()
		case <-closed:
		}
	}()
	return "http://" + listener.Addr().String(), closeProxy, nil
}

type sshDownloadProxy struct {
	dial       func(context.Context, string) (net.Conn, error)
	egressHost string
	trace      *SSHDownloadProxyTrace
}

func (p *sshDownloadProxy) serve(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go p.serveConnection(connection)
	}
}

func (p *sshDownloadProxy) serveConnection(client net.Conn) {
	defer client.Close()
	buffered := bufio.NewReader(client)
	r, err := http.ReadRequest(buffered)
	if err != nil {
		return
	}
	defer r.Body.Close()
	if r.Method == http.MethodConnect {
		p.connect(client, buffered, r)
		return
	}
	if r.URL.Scheme != "http" || !allowedProxyAddress(r.URL.Host, "80") {
		_, _ = io.WriteString(client, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	request := r.Clone(r.Context())
	request.RequestURI = ""
	request.Header.Del("Proxy-Connection")
	transport := &http.Transport{DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
		return p.dial(ctx, address)
	}}
	defer transport.CloseIdleConnections()
	response, err := transport.RoundTrip(request)
	if err != nil {
		log.Printf("[sing-box-install] egress=%s destination=%s connect failed: %v", p.egressHost, r.URL.Host, err)
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	defer response.Body.Close()
	response.Close = true
	_ = response.Write(client)
}

func (p *sshDownloadProxy) connect(client net.Conn, buffered *bufio.Reader, r *http.Request) {
	p.trace.record(fmt.Sprintf("egress=%s CONNECT %s received", p.egressHost, r.Host))
	if !allowedProxyAddress(r.Host, "443") {
		_, _ = io.WriteString(client, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	upstream, err := p.dial(r.Context(), r.Host)
	if err != nil {
		p.trace.record(fmt.Sprintf("egress=%s CONNECT %s failed: %v", p.egressHost, r.Host, err))
		log.Printf("[sing-box-install] egress=%s destination=%s connect failed: %v", p.egressHost, r.Host, err)
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	defer upstream.Close()
	p.trace.record(fmt.Sprintf("egress=%s CONNECT %s outbound ready", p.egressHost, r.Host))
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	p.trace.record(fmt.Sprintf("egress=%s CONNECT %s tunnel established", p.egressHost, r.Host))
	started := time.Now()
	var sentBytes, receivedBytes atomic.Int64
	stopProgress := make(chan struct{})
	defer close(stopProgress)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Printf("[sing-box-install] egress=%s CONNECT %s active after %s: request_bytes=%d response_bytes=%d", p.egressHost, r.Host, time.Since(started).Round(time.Second), sentBytes.Load(), receivedBytes.Load())
			case <-stopProgress:
				return
			}
		}
	}()
	requestBytes := make(chan int64, 1)
	go func() {
		bytes, _ := io.Copy(&countingWriter{Writer: upstream, count: &sentBytes}, buffered)
		requestBytes <- bytes
		_ = upstream.Close()
	}()
	responseBytes, _ := io.Copy(&countingWriter{Writer: client, count: &receivedBytes}, upstream)
	_ = client.Close()
	_ = upstream.Close()
	log.Printf("[sing-box-install] egress=%s CONNECT %s closed after %s: request_bytes=%d response_bytes=%d", p.egressHost, r.Host, time.Since(started).Round(time.Second), <-requestBytes, responseBytes)
}

type countingWriter struct {
	io.Writer
	count *atomic.Int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.count.Add(int64(n))
	return n, err
}

// A remote shell uses the same resolver and outbound network path as a curl
// command on the selected server. SSH direct-tcpip may be restricted by sshd.
func dialSSHOutbound(ctx context.Context, client *ssh.Client, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || (port != "80" && port != "443") {
		return nil, fmt.Errorf("unsupported outbound address %q", address)
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	if err := session.Start(sshOutboundCommand(host, port)); err != nil {
		_ = session.Close()
		return nil, err
	}
	ready := make(chan error, 1)
	go func() {
		line, readErr := bufio.NewReader(stderr).ReadString('\n')
		if readErr != nil {
			ready <- fmt.Errorf("出口节点无法连接 %s: %w", address, readErr)
		} else if !strings.HasPrefix(strings.TrimSpace(line), "CYLISM_PROXY_READY ") {
			ready <- fmt.Errorf("出口节点无法连接 %s: %s", address, strings.TrimSpace(line))
		} else {
			log.Printf("[sing-box-install] destination=%s outbound_ip=%s", address, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "CYLISM_PROXY_READY ")))
			ready <- nil
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			_ = session.Close()
			return nil, err
		}
	case <-timer.C:
		_ = session.Close()
		return nil, fmt.Errorf("出口节点连接 %s 超时", address)
	case <-ctx.Done():
		_ = session.Close()
		return nil, ctx.Err()
	}
	return &sshOutboundConn{Reader: stdout, WriteCloser: stdin, session: session}, nil
}

type sshOutboundConn struct {
	io.Reader
	io.WriteCloser
	session *ssh.Session
	once    sync.Once
}

func (c *sshOutboundConn) Read(p []byte) (int, error)  { return c.Reader.Read(p) }
func (c *sshOutboundConn) Write(p []byte) (int, error) { return c.WriteCloser.Write(p) }
func (c *sshOutboundConn) Close() error {
	var err error
	c.once.Do(func() {
		_ = c.WriteCloser.Close()
		err = c.session.Close()
	})
	return err
}
func (c *sshOutboundConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *sshOutboundConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *sshOutboundConn) SetDeadline(time.Time) error      { return errors.ErrUnsupported }
func (c *sshOutboundConn) SetReadDeadline(time.Time) error  { return errors.ErrUnsupported }
func (c *sshOutboundConn) SetWriteDeadline(time.Time) error { return errors.ErrUnsupported }

func quoteSSHArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func sshOutboundCommand(host, port string) string {
	command := `host=$1; port=$2; ip=$(getent ahostsv4 "$host" 2>/dev/null | awk 'NR==1 {print $1}') || true; ` +
		`test -n "$ip" || ip=$host; exec 3<>/dev/tcp/"$ip"/"$port" || exit 1; ` +
		`printf 'CYLISM_PROXY_READY %s\n' "$ip" >&2; cat <&3 & cat >&3; wait`
	return "bash -c " + quoteSSHArgument(command) + " bash " + quoteSSHArgument(host) + " " + quoteSSHArgument(port)
}

func allowedProxyAddress(address, defaultPort string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host, port = address, defaultPort
	}
	return host != "" && !strings.ContainsAny(host, " /\\@") && (port == "80" || port == "443") && !strings.HasPrefix(host, "[")
}
