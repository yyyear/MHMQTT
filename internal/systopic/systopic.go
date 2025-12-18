package systopic

import (
	"fmt"
	"sync"
	"time"
)

// SystemTopic 系统主题管理器
type SystemTopic struct {
	mu             sync.RWMutex
	stats          *Stats
	updateInterval time.Duration
	stopChan       chan struct{}
}

// Stats 统计信息
type Stats struct {
	StartTime           time.Time
	BytesReceived       uint64
	BytesSent           uint64
	MessagesReceived    uint64
	MessagesSent        uint64
	ClientsConnected    uint32
	ClientsDisconnected uint32
	MessagesStored      uint32
	MessagesDropped     uint32
}

// NewSystemTopic 创建系统主题管理器
func NewSystemTopic(updateInterval time.Duration) *SystemTopic {
	return &SystemTopic{
		stats: &Stats{
			StartTime: time.Now(),
		},
		updateInterval: updateInterval,
		stopChan:       make(chan struct{}),
	}
}

// Start 启动系统主题更新
func (s *SystemTopic) Start() {
	go s.updateLoop()
}

// Stop 停止系统主题更新
func (s *SystemTopic) Stop() {
	close(s.stopChan)
}

// GetStats 获取统计信息
func (s *SystemTopic) GetStats() *Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// IncrementBytesReceived 增加接收字节数
func (s *SystemTopic) IncrementBytesReceived(bytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.BytesReceived += bytes
}

// IncrementBytesSent 增加发送字节数
func (s *SystemTopic) IncrementBytesSent(bytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.BytesSent += bytes
}

// IncrementMessagesReceived 增加接收消息数
func (s *SystemTopic) IncrementMessagesReceived() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.MessagesReceived++
}

// IncrementMessagesSent 增加发送消息数
func (s *SystemTopic) IncrementMessagesSent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.MessagesSent++
}

// IncrementClientsConnected 增加连接客户端数
func (s *SystemTopic) IncrementClientsConnected() {
	s.stats.ClientsConnected++
}

// IncrementClientsDisconnected 增加断开客户端数
func (s *SystemTopic) IncrementClientsDisconnected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.ClientsDisconnected++
}

// IncrementMessagesStored 增加存储消息数
func (s *SystemTopic) IncrementMessagesStored() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.MessagesStored++
}

// IncrementMessagesDropped 增加丢弃消息数
func (s *SystemTopic) IncrementMessagesDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.MessagesDropped++
}

// SetClientsConnected 设置当前连接客户端数
func (s *SystemTopic) SetClientsConnected(count uint32) {
	s.stats.ClientsConnected = count
}

// GetSystemTopics 获取系统主题消息
func (s *SystemTopic) GetSystemTopics() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := s.stats
	uptime := time.Since(stats.StartTime)
	
	return map[string]string{
		"$SYS/broker/version":              "MHMQTT/1.0",
		"$SYS/broker/uptime":               fmt.Sprintf("%d", int(uptime.Seconds())),
		"$SYS/broker/timestamp":            fmt.Sprintf("%d", time.Now().Unix()),
		"$SYS/broker/clients/connected":    fmt.Sprintf("%d", stats.ClientsConnected),
		"$SYS/broker/clients/disconnected": fmt.Sprintf("%d", stats.ClientsDisconnected),
		"$SYS/broker/messages/received":    fmt.Sprintf("%d", stats.MessagesReceived),
		"$SYS/broker/messages/sent":        fmt.Sprintf("%d", stats.MessagesSent),
		"$SYS/broker/bytes/received":       fmt.Sprintf("%d", stats.BytesReceived),
		"$SYS/broker/bytes/sent":           fmt.Sprintf("%d", stats.BytesSent),
		"$SYS/broker/messages/stored":      fmt.Sprintf("%d", stats.MessagesStored),
		"$SYS/broker/messages/dropped":     fmt.Sprintf("%d", stats.MessagesDropped),
	}
}

// updateLoop 更新循环
func (s *SystemTopic) updateLoop() {
	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// 系统主题更新由 broker 处理
		case <-s.stopChan:
			return
		}
	}
}
