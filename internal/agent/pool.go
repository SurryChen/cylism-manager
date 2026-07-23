package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	pb "github.com/cylism/cylism-manager/api/proto/agent"
	"github.com/cylism/cylism-manager/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Pool gRPC 连接池，管理与多台服务器 Agent 的连接
type Pool struct {
	mu      sync.RWMutex
	clients map[uint]*AgentConn // serverID -> conn
	timeout time.Duration
}

// AgentConn 封装到一台 Agent 的连接
type AgentConn struct {
	ServerID uint
	Conn     *grpc.ClientConn
	Client   pb.AgentServiceClient
	LastSeen time.Time
}

// NewPool 创建连接池
func NewPool(timeout time.Duration) *Pool {
	return &Pool{
		clients: make(map[uint]*AgentConn),
		timeout: timeout,
	}
}

// Connect 建立到指定服务器的 gRPC 连接
func (p *Pool) Connect(server *model.Server) (*AgentConn, error) {
	return p.ConnectWithTLS(server, nil, nil, nil)
}

// ConnectWithTLS 建立到指定服务器的 mTLS gRPC 连接
func (p *Pool) ConnectWithTLS(server *model.Server, caCertPEM []byte, certPEM, keyPEM []byte) (*AgentConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 关闭已有连接
	if existing, ok := p.clients[server.ID]; ok {
		existing.Conn.Close()
	}

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)

	var creds credentials.TransportCredentials
	if len(certPEM) > 0 && len(keyPEM) > 0 && len(caCertPEM) > 0 {
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(caCertPEM)
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caPool,
			// 自签 CA 环境，证书 CN=agent-{id}，不匹配目标 IP，跳过主机名校验
			InsecureSkipVerify: true,
		}
		creds = credentials.NewTLS(tlsCfg)
	} else {
		creds = insecure.NewCredentials()
	}

	// 阻塞 Dial，等待 TCP 连接真正建立
	log.Printf("connect: dialing %s (timeout=%v, tls=%v)", addr, p.timeout, len(certPEM) > 0)
	dialStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	dialElapsed := time.Since(dialStart)
	if err != nil {
		log.Printf("connect: dial %s failed after %v: %v", addr, dialElapsed, err)
		return nil, fmt.Errorf("dial agent: %w", err)
	}
	log.Printf("connect: dial %s OK (%v), now pinging...", addr, dialElapsed)

	ac := &AgentConn{
		ServerID: server.ID,
		Conn:     conn,
		Client:   pb.NewAgentServiceClient(conn),
	}

	// Ping 验活：确认 Agent 真的在线，防止假连接
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()
	pingStart := time.Now()
	_, pingErr := ac.Client.Ping(pingCtx, &pb.PingRequest{})
	pingElapsed := time.Since(pingStart)
	if pingErr != nil {
		log.Printf("connect: ping %s failed after %v: %v", addr, pingElapsed, pingErr)
		conn.Close()
		return nil, fmt.Errorf("agent unreachable after connect: %w", pingErr)
	}
	log.Printf("connect: ping %s OK (%v)", addr, pingElapsed)
	ac.LastSeen = time.Now()

	p.clients[server.ID] = ac
	return ac, nil
}

// Reconnect 尝试重新连接指定服务器，自动加载 mTLS 证书
func (p *Pool) Reconnect(server *model.Server) (*AgentConn, error) {
	certDir := fmt.Sprintf("data/tls/servers")
	certPEM, _ := os.ReadFile(fmt.Sprintf("%s/%d-cert.pem", certDir, server.ID))
	keyPEM, _ := os.ReadFile(fmt.Sprintf("%s/%d-key.pem", certDir, server.ID))
	caPEM, _ := os.ReadFile("data/tls/ca-cert.pem")
	return p.ConnectWithTLS(server, caPEM, certPEM, keyPEM)
}

// Get 获取指定服务器的 Agent 客户端
func (p *Pool) Get(serverID uint) (*AgentConn, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ac, ok := p.clients[serverID]
	return ac, ok
}

// Close 关闭指定连接
func (p *Pool) Close(serverID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ac, ok := p.clients[serverID]; ok {
		ac.Conn.Close()
		delete(p.clients, serverID)
	}
}

// CloseAll 关闭所有连接
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ac := range p.clients {
		ac.Conn.Close()
		delete(p.clients, id)
	}
}

// Ping 向指定 Agent 发送心跳
func (p *Pool) Ping(ctx context.Context, serverID uint) error {
	ac, ok := p.Get(serverID)
	if !ok {
		return fmt.Errorf("server %d not connected", serverID)
	}

	resp, err := ac.Client.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("agent returned not ok")
	}
	ac.LastSeen = time.Now()
	return nil
}

// Connected 返回当前连接数
func (p *Pool) Connected() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients)
}
