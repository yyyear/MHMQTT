package pool

import (
	"context"
	"sync"
	"time"
)

// ConnectionPool 连接池
type ConnectionPool struct {
	mu      sync.RWMutex
	clients map[string]interface{} // clientID -> client
	maxSize int
	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewConnectionPool 创建连接池
func NewConnectionPool(maxSize int, timeout time.Duration) *ConnectionPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &ConnectionPool{
		clients: make(map[string]interface{}),
		maxSize: maxSize,
		timeout: timeout,
		ctx:     ctx,
		cancel:  cancel,
	}
	
	// 启动清理协程
	go pool.cleanup()
	
	return pool
}

// Add 添加连接
func (p *ConnectionPool) Add(clientID string, client interface{}) bool {
	if len(p.clients) >= p.maxSize {
		return false
	}
	
	p.clients[clientID] = client
	return true
}

// Get 获取连接
func (p *ConnectionPool) Get(clientID string) (interface{}, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	client, ok := p.clients[clientID]
	return client, ok
}

// Remove 移除连接
func (p *ConnectionPool) Remove(clientID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	delete(p.clients, clientID)
}

// Size 获取连接数
func (p *ConnectionPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return len(p.clients)
}

// GetAll 获取所有连接
func (p *ConnectionPool) GetAll() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	result := make(map[string]interface{})
	for k, v := range p.clients {
		result[k] = v
	}
	return result
}

// cleanup 清理超时连接
func (p *ConnectionPool) cleanup() {
	ticker := time.NewTicker(p.timeout / 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			// 清理逻辑由外部实现
		}
	}
}

// Close 关闭连接池
func (p *ConnectionPool) Close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.clients = make(map[string]interface{})
}
