package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/cylism/cylism-manager/api/proto/agent"
	"github.com/cylism/cylism-manager/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	p.mu.Lock()
	defer p.mu.Unlock()

	// 关闭已有连接
	if existing, ok := p.clients[server.ID]; ok {
		existing.Conn.Close()
	}

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(p.timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("dial agent: %w", err)
	}

	ac := &AgentConn{
		ServerID: server.ID,
		Conn:     conn,
		Client:   pb.NewAgentServiceClient(conn),
		LastSeen: time.Now(),
	}
	p.clients[server.ID] = ac
	return ac, nil
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
