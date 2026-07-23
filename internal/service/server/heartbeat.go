package server

import (
	"context"
	"log"
	"time"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/store"
)

// HeartbeatMonitor 后台心跳监控
type HeartbeatMonitor struct {
	store    *store.Store
	pool     *agent.Pool
	interval time.Duration
	stopCh   chan struct{}
}

// NewHeartbeatMonitor 创建心跳监控器
func NewHeartbeatMonitor(s *store.Store, pool *agent.Pool, interval time.Duration) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		store:    s,
		pool:     pool,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动心跳监控
func (m *HeartbeatMonitor) Start() {
	go m.loop()
}

// Stop 停止心跳监控
func (m *HeartbeatMonitor) Stop() {
	close(m.stopCh)
}

func (m *HeartbeatMonitor) loop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	failCount := make(map[uint]int) // serverID -> 连续失败次数

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAll(failCount)
		}
	}
}

func (m *HeartbeatMonitor) checkAll(failCount map[uint]int) {
	servers, err := m.store.ListServers()
	if err != nil {
		log.Printf("heartbeat: list servers: %v", err)
		return
	}

	for _, srv := range servers {
		if srv.Status != "online" {
			continue
		}

		ac, ok := m.pool.Get(srv.ID)
		if !ok {
			failCount[srv.ID]++
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := m.pool.Ping(ctx, srv.ID)
			cancel()

			if err != nil {
				failCount[srv.ID]++
			} else {
				failCount[srv.ID] = 0
				now := time.Now()
				srv.LastSeen = &now
				m.store.UpdateServer(&srv)
			}
		}
		_ = ac

		// 连续 3 次失败标记为 offline
		if failCount[srv.ID] >= 3 {
			srv.Status = "offline"
			m.store.UpdateServer(&srv)
			m.pool.Close(srv.ID)
			delete(failCount, srv.ID)
		}
	}
}
