package topic

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

// subscriberCacheEntry 订阅者缓存条目
type subscriberCacheEntry struct {
	subscribers map[string]byte
	expireAt    int64 // Unix 纳秒时间戳
	hitCount    uint64
}

// Manager 主题管理器
type Manager struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]byte            // topic -> clientID -> QoS
	sharedSubs  map[string]map[string]map[string]byte // group -> filter -> clientID -> QoS
	rrIndex     map[string]int                        // round-robin index per group|filter
	rrMu        sync.Mutex
	matcher     Matcher

	// 高频主题订阅者缓存
	cacheMu       sync.RWMutex
	cache         map[string]*subscriberCacheEntry // topic -> cached subscribers
	cacheVersion  uint64                           // 缓存版本，订阅变更时递增
	cacheTTL      time.Duration                    // 缓存过期时间
	cacheMaxSize  int                              // 最大缓存条目数
	cacheHitMin   uint64                           // 最小命中次数阈值，超过此值才缓存
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewManager 创建新的主题管理器
func NewManager() *Manager {
	return NewManagerWithCache(100*time.Millisecond, 10000000, 3)
}

// NewManagerWithCache 创建带缓存配置的主题管理器
func NewManagerWithCache(cacheTTL time.Duration, cacheMaxSize int, cacheHitMin uint64) *Manager {
	m := &Manager{
		subscribers:  make(map[string]map[string]byte),
		sharedSubs:   make(map[string]map[string]map[string]byte),
		rrIndex:      make(map[string]int),
		matcher:      NewTrieMatcher(),
		cache:        make(map[string]*subscriberCacheEntry),
		cacheTTL:     cacheTTL,
		cacheMaxSize: cacheMaxSize,
		cacheHitMin:  cacheHitMin,
		stopCleanup:  make(chan struct{}),
	}

	// 启动缓存清理协程
	m.cleanupTicker = time.NewTicker(cacheTTL * 2)
	go m.cacheCleanup()

	return m
}

// cacheCleanup 定期清理过期缓存
func (m *Manager) cacheCleanup() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanExpiredCache()
		case <-m.stopCleanup:
			m.cleanupTicker.Stop()
			return
		}
	}
}

// cleanExpiredCache 清理过期缓存条目
func (m *Manager) cleanExpiredCache() {
	now := time.Now().UnixNano()

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	for topic, entry := range m.cache {
		if entry.expireAt < now {
			delete(m.cache, topic)
		}
	}
}

// invalidateCache 使缓存失效
func (m *Manager) invalidateCache() {
	atomic.AddUint64(&m.cacheVersion, 1)
	m.cacheMu.Lock()
	m.cache = make(map[string]*subscriberCacheEntry)
	m.cacheMu.Unlock()
}

// Stop 停止管理器（清理资源）
func (m *Manager) Stop() {
	close(m.stopCleanup)
}

// isSharedTopic 判断是否为共享订阅
// 支持:
// - $share/<group>/<filter>
// - $queue/<filter> (作为 $share/queue/<filter> 的别名)
func isSharedTopic(topic string) (group, filter string, ok bool) {
	if strings.HasPrefix(topic, "$share/") {
		parts := strings.SplitN(topic, "/", 3)
		if len(parts) == 3 && parts[2] != "" {
			return parts[1], parts[2], true
		}
		return "", "", false
	}
	if strings.HasPrefix(topic, "$queue/") {
		filter := strings.TrimPrefix(topic, "$queue/")
		if filter != "" {
			return "queue", filter, true
		}
	}
	return "", "", false
}

// Subscribe 订阅主题
func (m *Manager) Subscribe(clientID, topic string, qos byte) {
	m.mu.Lock()

	if group, filter, shared := isSharedTopic(topic); shared {
		if !m.matcher.Validate(filter) {
			m.mu.Unlock()
			return
		}
		if m.sharedSubs[group] == nil {
			m.sharedSubs[group] = make(map[string]map[string]byte)
		}
		if m.sharedSubs[group][filter] == nil {
			m.sharedSubs[group][filter] = make(map[string]byte)
		}
		m.sharedSubs[group][filter][clientID] = qos
		m.mu.Unlock()
		// 使缓存失效
		m.invalidateCache()
		return
	}

	if !m.matcher.Validate(topic) {
		m.mu.Unlock()
		return
	}

	if m.subscribers[topic] == nil {
		m.subscribers[topic] = make(map[string]byte)
	}
	m.subscribers[topic][clientID] = qos
	m.mu.Unlock()

	// 使缓存失效
	m.invalidateCache()
}

// Unsubscribe 取消订阅
func (m *Manager) Unsubscribe(clientID, topic string) {
	var keyToDelete string
	var changed bool

	m.mu.Lock()
	if group, filter, shared := isSharedTopic(topic); shared {
		if groups, ok := m.sharedSubs[group]; ok {
			if subs, ok := groups[filter]; ok {
				if _, exists := subs[clientID]; exists {
					delete(subs, clientID)
					changed = true
				}
				if len(subs) == 0 {
					delete(groups, filter)
					// 记录需要删除的 key，在释放 mu 后再删除
					keyToDelete = group + "|" + filter
				}
			}
			if len(groups) == 0 {
				delete(m.sharedSubs, group)
			}
		}
		m.mu.Unlock()
		// 在释放 mu 之后再获取 rrMu，避免嵌套锁
		if keyToDelete != "" {
			m.rrMu.Lock()
			delete(m.rrIndex, keyToDelete)
			m.rrMu.Unlock()
		}
		// 使缓存失效
		if changed {
			m.invalidateCache()
		}
		return
	}

	if subscribers, ok := m.subscribers[topic]; ok {
		if _, exists := subscribers[clientID]; exists {
			delete(subscribers, clientID)
			changed = true
		}
		if len(subscribers) == 0 {
			delete(m.subscribers, topic)
		}
	}
	m.mu.Unlock()

	// 使缓存失效
	if changed {
		m.invalidateCache()
	}
}

// UnsubscribeAll 取消客户端的所有订阅
func (m *Manager) UnsubscribeAll(clientID string) {
	var changed bool

	m.mu.Lock()
	for topic, subscribers := range m.subscribers {
		if _, exists := subscribers[clientID]; exists {
			delete(subscribers, clientID)
			changed = true
		}
		if len(subscribers) == 0 {
			delete(m.subscribers, topic)
		}
	}

	// 也清理共享订阅
	for _, filters := range m.sharedSubs {
		for _, subs := range filters {
			if _, exists := subs[clientID]; exists {
				delete(subs, clientID)
				changed = true
			}
		}
	}
	m.mu.Unlock()

	// 使缓存失效
	if changed {
		m.invalidateCache()
	}
}

// GetSubscribers 获取订阅者（带缓存）
func (m *Manager) GetSubscribers(topic string) map[string]byte {
	now := time.Now().UnixNano()

	// 先检查缓存（对于非共享订阅的普通主题）
	m.cacheMu.RLock()
	if entry, ok := m.cache[topic]; ok && entry.expireAt > now {
		// 缓存命中，增加命中计数
		atomic.AddUint64(&entry.hitCount, 1)
		// 返回缓存的副本
		result := make(map[string]byte, len(entry.subscribers))
		for k, v := range entry.subscribers {
			result[k] = v
		}
		m.cacheMu.RUnlock()
		return result
	}
	m.cacheMu.RUnlock()

	// 缓存未命中，计算订阅者
	subscribers := m.computeSubscribers(topic)

	// 将结果放入缓存（只缓存非空结果且缓存未满）
	if len(subscribers) > 0 {
		m.cacheMu.Lock()
		if len(m.cache) < m.cacheMaxSize {
			// 创建副本存入缓存
			cached := make(map[string]byte, len(subscribers))
			for k, v := range subscribers {
				cached[k] = v
			}
			m.cache[topic] = &subscriberCacheEntry{
				subscribers: cached,
				expireAt:    now + int64(m.cacheTTL),
				hitCount:    1,
			}
		}
		m.cacheMu.Unlock()
	}

	return subscribers
}

// computeSubscribers 计算订阅者（无缓存）
func (m *Manager) computeSubscribers(topic string) map[string]byte {
	// 使用预分配的 map 减少扩容
	subscribers := make(map[string]byte, 16)

	// 快照拷贝，尽快释放读锁，避免阻塞写操作
	m.mu.RLock()

	// 精确匹配
	if subs, ok := m.subscribers[topic]; ok {
		for clientID, qos := range subs {
			subscribers[clientID] = qos
		}
	}

	// 复用切片来存储需要匹配的模式
	patterns := make([]struct {
		pattern string
		subs    map[string]byte
	}, 0, len(m.subscribers))

	for pattern, subs := range m.subscribers {
		if pattern != topic && (containsWildcard(pattern)) {
			// 拷贝内层 map
			cp := make(map[string]byte, len(subs))
			for cid, qos := range subs {
				cp[cid] = qos
			}
			patterns = append(patterns, struct {
				pattern string
				subs    map[string]byte
			}{pattern, cp})
		}
	}

	// 复制共享订阅数据
	shared := make([]struct {
		group  string
		filter string
		subs   map[string]byte
	}, 0, 8)

	for group, filters := range m.sharedSubs {
		for filter, subs := range filters {
			cp := make(map[string]byte, len(subs))
			for cid, qos := range subs {
				cp[cid] = qos
			}
			shared = append(shared, struct {
				group  string
				filter string
				subs   map[string]byte
			}{group, filter, cp})
		}
	}
	m.mu.RUnlock()

	// 通配符匹配
	for _, p := range patterns {
		if m.matcher.Match(topic, p.pattern) {
			for clientID, qos := range p.subs {
				if existingQoS, ok := subscribers[clientID]; !ok || qos > existingQoS {
					subscribers[clientID] = qos
				}
			}
		}
	}

	// 共享订阅匹配
	for _, s := range shared {
		if m.matcher.Match(topic, s.filter) && len(s.subs) > 0 {
			clientIDs := make([]string, 0, len(s.subs))
			for cid := range s.subs {
				clientIDs = append(clientIDs, cid)
			}
			// 排序以保证一致性
			for i := 1; i < len(clientIDs); i++ {
				j := i
				for j > 0 && clientIDs[j-1] > clientIDs[j] {
					clientIDs[j-1], clientIDs[j] = clientIDs[j], clientIDs[j-1]
					j--
				}
			}
			key := s.group + "|" + s.filter
			m.rrMu.Lock()
			idx := m.rrIndex[key] % len(clientIDs)
			selected := clientIDs[idx]
			m.rrIndex[key] = (m.rrIndex[key] + 1) % len(clientIDs)
			m.rrMu.Unlock()
			qos := s.subs[selected]
			if existingQoS, ok := subscribers[selected]; !ok || qos > existingQoS {
				subscribers[selected] = qos
			}
		}
	}

	return subscribers
}

// containsWildcard 检查主题是否包含通配符
func containsWildcard(topic string) bool {
	return strings.ContainsAny(topic, "+#")
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
