package topic

import (
	"strings"
	"sync"
)

// Matcher 主题匹配器
type Matcher interface {
	Match(topic, pattern string) bool
	Validate(topic string) bool
}

// TrieMatcher 基于 Trie 的主题匹配器
type TrieMatcher struct {
	mu sync.RWMutex
}

// NewTrieMatcher 创建新的主题匹配器
func NewTrieMatcher() *TrieMatcher {
	return &TrieMatcher{}
}

// Match 匹配主题和模式
// 支持 + 和 # 通配符
func (m *TrieMatcher) Match(topic, pattern string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.match(topic, pattern)
}

func (m *TrieMatcher) match(topic, pattern string) bool {
	topicParts := strings.Split(topic, "/")
	patternParts := strings.Split(pattern, "/")
	
	return m.matchParts(topicParts, patternParts)
}

func (m *TrieMatcher) matchParts(topicParts, patternParts []string) bool {
	if len(patternParts) == 0 {
		return len(topicParts) == 0
	}
	
	if len(topicParts) == 0 {
		// 只有 # 可以匹配空
		return len(patternParts) == 1 && patternParts[0] == "#"
	}
	
	patternPart := patternParts[0]
	topicPart := topicParts[0]
	
	switch patternPart {
	case "#":
		// # 必须位于最后
		return len(patternParts) == 1
	case "+":
		// + 匹配单级
		return m.matchParts(topicParts[1:], patternParts[1:])
	default:
		// 精确匹配
		if patternPart != topicPart {
			return false
		}
		return m.matchParts(topicParts[1:], patternParts[1:])
	}
}

// Validate 验证主题格式
func (m *TrieMatcher) Validate(topic string) bool {
	if topic == "" {
		return false
	}
	
	// 不能包含空字符串段
	parts := strings.Split(topic, "/")
	for _, part := range parts {
		if part == "" && len(parts) > 1 {
			// 允许前导或尾随的 /，但不允许中间的 //
			return false
		}
	}
	
	// 检查通配符使用
	hasHash := false
	for i, part := range parts {
		if part == "#" {
			// # 必须位于最后
			if i != len(parts)-1 {
				return false
			}
			hasHash = true
		}
		if part == "+" && hasHash {
			return false
		}
	}
	
	return true
}

// Manager 主题管理器
type Manager struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]byte // topic -> clientID -> QoS
	matcher     Matcher
}

// NewManager 创建新的主题管理器
func NewManager() *Manager {
	return &Manager{
		subscribers: make(map[string]map[string]byte),
		matcher:     NewTrieMatcher(),
	}
}

// Subscribe 订阅主题
func (m *Manager) Subscribe(clientID, topic string, qos byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.matcher.Validate(topic) {
		return
	}
	
	if m.subscribers[topic] == nil {
		m.subscribers[topic] = make(map[string]byte)
	}
	m.subscribers[topic][clientID] = qos
}

// Unsubscribe 取消订阅
func (m *Manager) Unsubscribe(clientID, topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if subscribers, ok := m.subscribers[topic]; ok {
		delete(subscribers, clientID)
		if len(subscribers) == 0 {
			delete(m.subscribers, topic)
		}
	}
}

// UnsubscribeAll 取消客户端的所有订阅
func (m *Manager) UnsubscribeAll(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for topic, subscribers := range m.subscribers {
		delete(subscribers, clientID)
		if len(subscribers) == 0 {
			delete(m.subscribers, topic)
		}
	}
}

// GetSubscribers 获取订阅者
func (m *Manager) GetSubscribers(topic string) map[string]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	subscribers := make(map[string]byte)
	
	// 精确匹配
	if subs, ok := m.subscribers[topic]; ok {
		for clientID, qos := range subs {
			subscribers[clientID] = qos
		}
	}
	
	// 通配符匹配
	for pattern, subs := range m.subscribers {
		if pattern != topic && m.matcher.Match(topic, pattern) {
			for clientID, qos := range subs {
				// 如果已存在，取较大的 QoS
				if existingQoS, ok := subscribers[clientID]; !ok || qos > existingQoS {
					subscribers[clientID] = qos
				}
			}
		}
	}
	
	return subscribers
}

// GetSubscriptions 获取客户端的订阅列表
func (m *Manager) GetSubscriptions(clientID string) map[string]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	subscriptions := make(map[string]byte)
	for topic, subscribers := range m.subscribers {
		if qos, ok := subscribers[clientID]; ok {
			subscriptions[topic] = qos
		}
	}
	
	return subscriptions
}

// IsSystemTopic 检查是否是系统主题
func IsSystemTopic(topic string) bool {
	return strings.HasPrefix(topic, "$SYS/")
}

