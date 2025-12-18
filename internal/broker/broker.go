package broker

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
	
	"mhmqtt/internal/client"
	"mhmqtt/internal/cluster"
	"mhmqtt/internal/config"
	"mhmqtt/internal/message"
	"mhmqtt/internal/pool"
	"mhmqtt/internal/protocol"
	"mhmqtt/internal/storage"
	"mhmqtt/internal/systopic"
	"mhmqtt/internal/topic"
	
	"github.com/yyyear/YY"
)

// Broker MQTT Broker
type Broker struct {
	config *config.Config
	
	// 组件
	storage      storage.Storage
	topicManager *topic.Manager
	clientPool   *pool.ConnectionPool
	sysTopic     *systopic.SystemTopic
	cluster      *cluster.Cluster
	
	// 状态
	mu         sync.RWMutex
	clients    map[string]*client.Client
	listener   net.Listener
	wsListener net.Listener
	running    bool
	
	// 批处理
	batchProcessor *message.BatchProcessor
}

// NewBroker 创建新的 Broker
func NewBroker(cfg *config.Config) *Broker {
	// 创建存储
	stor, err := storage.NewBoltStorage(cfg.Broker.DataPath + "/broker.db")
	if err != nil {
		log.Printf("警告: 创建存储失败: %v", err)
	}
	
	b := &Broker{
		config:       cfg,
		storage:      stor,
		topicManager: topic.NewManager(),
		clients:      make(map[string]*client.Client),
		sysTopic:     systopic.NewSystemTopic(10 * time.Second),
	}
	
	// 创建连接池
	b.clientPool = pool.NewConnectionPool(
		cfg.Broker.MaxClients,
		time.Duration(cfg.Broker.KeepAlive)*time.Second,
	)
	
	// 创建批处理器
	b.batchProcessor = message.NewBatchProcessor(
		100,                  // 批大小
		100*time.Millisecond, // 批窗口
		b.processBatch,
	)
	b.batchProcessor.Start()
	
	// 创建集群
	if cfg.Broker.Cluster.Enabled {
		b.cluster = cluster.NewCluster(cfg.Broker.Cluster, b)
	}
	
	return b
}

// Start 启动 Broker
func (b *Broker) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.running {
		return fmt.Errorf("broker 已在运行")
	}
	
	// 启动系统主题
	b.sysTopic.Start()
	
	// 启动集群
	if b.cluster != nil {
		if err := b.cluster.Start(); err != nil {
			return fmt.Errorf("启动集群失败: %w", err)
		}
	}
	
	// 启动 MQTT 监听
	listener, err := net.Listen("tcp", b.config.Broker.Address)
	if err != nil {
		return fmt.Errorf("监听 MQTT 端口失败: %w", err)
	}
	b.listener = listener
	
	// 启动 WebSocket 监听（简化处理，使用普通 TCP）
	// 实际实现中需要使用 gorilla/websocket
	
	b.running = true
	
	// 启动接受连接
	go b.acceptConnections()
	
	// 启动 Keep Alive 检查
	go b.keepAliveChecker()
	
	return nil
}

// acceptConnections 接受连接
func (b *Broker) acceptConnections() {
	for {
		conn, err := b.listener.Accept()
		log.Println("接受连接:", conn.RemoteAddr())
		if err != nil {
			if b.isRunning() {
				log.Printf("接受连接失败: %v", err)
			}
			return
		}
		
		go b.handleConnection(conn)
	}
}

// handleConnection 处理连接
func (b *Broker) handleConnection(conn net.Conn) {
	defer conn.Close()
	YY.Debug("处理连接:", conn.RemoteAddr())
	// 读取 CONNECT 消息
	reader := protocol.NewPacketReader(conn, b.config.Broker.MaxMessageSize)
	packet, err := reader.ReadPacket()
	if err != nil {
		return
	}
	YY.Debug("读取 CONNECT 消息:", string(packet))
	
	// 解析 CONNECT
	connectMsg, err := protocol.ParseConnect(packet)
	if err != nil {
		YY.Error("解析 CONNECT 消息失败:", err, connectMsg)
		return
	}
	YY.Debug("解析 CONNECT 消息:", connectMsg)
	
	// 验证协议版本
	if connectMsg.ProtocolVersion != protocol.Version31 &&
		connectMsg.ProtocolVersion != protocol.Version311 &&
		connectMsg.ProtocolVersion != protocol.Version50 {
		// 发送不支持的协议版本
		return
	}
	YY.Debug("验证协议版本:", connectMsg.ProtocolVersion)
	
	// 检查客户端 ID
	if connectMsg.ClientID == "" {
		if connectMsg.ProtocolVersion == protocol.Version50 {
			// v5.0 可以自动生成 ClientID
			connectMsg.ClientID = fmt.Sprintf("auto-%d", time.Now().UnixNano())
		} else {
			// v3.1.1 需要 ClientID
			return
		}
	}
	YY.Debug("检查客户端 ID:", connectMsg.ClientID)
	
	// 检查是否已存在连接
	
	existingClient, exists := b.clients[connectMsg.ClientID]
	if exists && existingClient.IsConnected() {
		// 断开旧连接
		b.mu.Lock()
		existingClient.Disconnect()
		b.mu.Unlock()
	}
	
	YY.Debug("检查是否已存在连接:", existingClient)
	
	// 创建客户端
	c := client.NewClient(conn, b.storage)
	c.ID = connectMsg.ClientID
	c.Version = connectMsg.ProtocolVersion
	c.KeepAlive = connectMsg.KeepAlive
	c.CleanSession = connectMsg.CleanSession
	YY.Debug("设置 CleanSession:", c.CleanSession)
	
	// 处理 Will 消息
	if connectMsg.WillFlag {
		c.WillMessage = &protocol.Message{
			Topic:   connectMsg.WillTopic,
			Payload: connectMsg.WillMessage,
			QoS:     connectMsg.WillQoS,
			Retain:  connectMsg.WillRetain,
		}
	}
	YY.Debug("处理 Will 消息:", c.WillMessage)
	
	// 设置回调
	c.SetCallbacks(
		b.onClientDisconnect,
		b.onPublish,
		b.onSubscribe,
		b.onUnsubscribe,
		b.onPing,
	)
	YY.Debug("设置回调:", c)
	
	// 恢复会话
	sessionPresent := false
	if !c.CleanSession && b.storage != nil {
		sess, err := b.storage.GetSession(c.ID)
		if err == nil && sess != nil {
			sessionPresent = true
			// 恢复订阅
			for topic, qos := range sess.Subscriptions {
				b.topicManager.Subscribe(c.ID, topic, qos)
			}
			// 发送离线消息
			go b.sendOfflineMessages(c)
		}
		YY.Debug("恢复会话:", sess)
	}
	
	// 发送 CONNACK
	var returnCode byte
	if c.Version == protocol.Version50 {
		returnCode = protocol.ConnAckV5Success
	} else {
		returnCode = protocol.ConnAckAccepted
	}
	if err := c.SendConnAck(returnCode, sessionPresent); err != nil {
		return
	}
	YY.Debug("发送 CONNACK:", returnCode, sessionPresent)
	
	// 保存会话
	if b.storage != nil {
		sess := &storage.Session{
			ClientID:      c.ID,
			CleanSession:  c.CleanSession,
			Subscriptions: b.topicManager.GetSubscriptions(c.ID),
			WillMessage:   c.WillMessage,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		b.storage.SaveSession(sess)
	}
	YY.Debug("开始添加到连接池:", c.ID)
	// 添加到连接池
	b.mu.Lock()
	b.clients[c.ID] = c
	b.clientPool.Add(c.ID, c)
	b.sysTopic.IncrementClientsConnected()
	b.sysTopic.SetClientsConnected(uint32(len(b.clients)))
	b.mu.Unlock()
	log.Println("客户端连接:", c.ID)
	// 启动客户端处理
	c.Start()
}

// onClientDisconnect 客户端断开连接
func (b *Broker) onClientDisconnect(c *client.Client) {
	b.mu.Lock()
	delete(b.clients, c.ID)
	b.clientPool.Remove(c.ID)
	b.sysTopic.IncrementClientsDisconnected()
	b.sysTopic.SetClientsConnected(uint32(len(b.clients)))
	b.mu.Unlock()
	
	// 发送 Will 消息
	if c.WillMessage != nil {
		b.publishMessage(c.WillMessage)
	}
	
	// 清理会话
	if c.CleanSession && b.storage != nil {
		b.storage.DeleteSession(c.ID)
		b.topicManager.UnsubscribeAll(c.ID)
	}
}

// onPublish 处理发布消息
func (b *Broker) onPublish(c *client.Client, msg *protocol.PublishMessage) {
	b.sysTopic.IncrementMessagesReceived()
	b.sysTopic.IncrementBytesReceived(uint64(len(msg.Payload)))
	
	// 创建消息
	pubMsg := &protocol.Message{
		Topic:    msg.Topic,
		Payload:  msg.Payload,
		QoS:      msg.QoS,
		Retain:   msg.Retain,
		PacketID: msg.PacketID,
	}
	
	// 保存 Retained 消息
	if msg.Retain {
		if len(msg.Payload) == 0 {
			// 删除 Retained 消息
			if b.storage != nil {
				b.storage.DeleteRetainedMessage(msg.Topic)
			}
		} else {
			// 保存 Retained 消息
			if b.storage != nil {
				b.storage.SaveRetainedMessage(msg.Topic, pubMsg)
			}
		}
	}
	
	// 发布消息
	b.publishMessage(pubMsg)
}

// publishMessage 发布消息
func (b *Broker) publishMessage(msg *protocol.Message) {
	// 获取订阅者
	subscribers := b.topicManager.GetSubscribers(msg.Topic)
	
	// 发送给订阅者
	for clientID, qos := range subscribers {
		b.mu.RLock()
		c, ok := b.clients[clientID]
		b.mu.RUnlock()
		
		if !ok {
			// 客户端离线，保存离线消息
			if qos > 0 && b.storage != nil {
				offlineMsg := *msg
				// 为离线消息生成新的 PacketID
				offlineMsg.PacketID = uint16(time.Now().UnixNano() & 0xFFFF)
				b.storage.SaveOfflineMessage(clientID, &offlineMsg)
			}
			continue
		}
		
		// 发送消息
		pubMsg := &protocol.PublishMessage{
			Topic:    msg.Topic,
			Payload:  msg.Payload,
			QoS:      qos,
			Retain:   false, // 转发时不保留
			PacketID: c.NextPacketID(),
		}
		
		if err := c.SendPublish(pubMsg); err != nil {
			// 发送失败，保存离线消息
			if qos > 0 && b.storage != nil {
				b.storage.SaveOfflineMessage(clientID, msg)
			}
		} else {
			b.sysTopic.IncrementMessagesSent()
			b.sysTopic.IncrementBytesSent(uint64(len(msg.Payload)))
		}
	}
	
	// 集群转发
	if b.cluster != nil {
		b.cluster.ForwardMessage(msg)
	}
}

// onSubscribe 处理订阅
func (b *Broker) onSubscribe(c *client.Client, msg *protocol.SubscribeMessage) {
	returnCodes := make([]byte, len(msg.Topics))
	
	for i, sub := range msg.Topics {
		// 订阅主题
		b.topicManager.Subscribe(c.ID, sub.Topic, sub.QoS)
		returnCodes[i] = sub.QoS
		
		// 发送 Retained 消息：支持通配符与共享订阅
		if b.storage != nil {
			filter := sub.Topic
			if strings.HasPrefix(filter, "$share/") || strings.HasPrefix(filter, "$queue/") {
				parts := strings.SplitN(filter, "/", 3)
				if strings.HasPrefix(filter, "$queue/") {
					parts = []string{"$share", "queue", strings.TrimPrefix(filter, "$queue/")}
				}
				if len(parts) == 3 {
					filter = parts[2]
				}
			}
			if filter != "" {
				retainedAll, err := b.storage.GetRetainedMessages()
				if err == nil && retainedAll != nil {
					matcher := topic.NewTrieMatcher()
					for rTopic, retained := range retainedAll {
						if matcher.Match(rTopic, filter) {
							pubMsg := &protocol.PublishMessage{
								Topic:    retained.Topic,
								Payload:  retained.Payload,
								QoS:      sub.QoS,
								Retain:   true,
								PacketID: c.NextPacketID(),
							}
							c.SendPublish(pubMsg)
						}
					}
				}
			}
		}
	}
	
	// 发送 SUBACK
	c.SendSubAck(msg.PacketID, returnCodes)
	
	// 更新会话
	if b.storage != nil {
		sess := &storage.Session{
			ClientID:      c.ID,
			CleanSession:  c.CleanSession,
			Subscriptions: b.topicManager.GetSubscriptions(c.ID),
			UpdatedAt:     time.Now(),
		}
		b.storage.SaveSession(sess)
	}
}

// onUnsubscribe 处理取消订阅
func (b *Broker) onUnsubscribe(c *client.Client, msg *protocol.UnsubscribeMessage) {
	for _, topicName := range msg.Topics {
		b.topicManager.Unsubscribe(c.ID, topicName)
	}
	
	// 发送 UNSUBACK
	c.SendUnsubAck(msg.PacketID)
	
	// 更新会话
	if b.storage != nil {
		sess := &storage.Session{
			ClientID:      c.ID,
			CleanSession:  c.CleanSession,
			Subscriptions: b.topicManager.GetSubscriptions(c.ID),
			UpdatedAt:     time.Now(),
		}
		b.storage.SaveSession(sess)
	}
}

// onPing 处理 PING
func (b *Broker) onPing(c *client.Client) {
	// 由客户端处理
}

// sendOfflineMessages 发送离线消息
func (b *Broker) sendOfflineMessages(c *client.Client) {
	if b.storage == nil {
		return
	}
	
	messages, err := b.storage.GetOfflineMessages(c.ID)
	if err != nil {
		return
	}
	
	for _, msg := range messages {
		pubMsg := &protocol.PublishMessage{
			Topic:    msg.Topic,
			Payload:  msg.Payload,
			QoS:      msg.QoS,
			Retain:   false,
			PacketID: c.NextPacketID(),
		}
		
		if err := c.SendPublish(pubMsg); err != nil {
			break
		}
		
		// 删除离线消息
		b.storage.DeleteOfflineMessage(c.ID, msg.PacketID)
	}
}

// keepAliveChecker Keep Alive 检查
func (b *Broker) keepAliveChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if !b.isRunning() {
				return
			}
			
			b.mu.RLock()
			clients := make([]*client.Client, 0, len(b.clients))
			for _, c := range b.clients {
				clients = append(clients, c)
			}
			b.mu.RUnlock()
			
			for _, c := range clients {
				if c.KeepAlive > 0 {
					lastPing := c.GetLastPing()
					timeout := time.Duration(c.KeepAlive) * time.Second * 3 / 2
					if time.Since(lastPing) > timeout {
						c.Disconnect()
					}
				}
			}
		}
	}
}

// processBatch 处理批处理
func (b *Broker) processBatch(batch []interface{}) {
	// 批处理逻辑
	for _, item := range batch {
		// 处理消息
		_ = item
	}
}

// isRunning 检查是否运行中
func (b *Broker) isRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

// Stop 停止 Broker
func (b *Broker) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if !b.running {
		return
	}
	
	b.running = false
	
	// 停止监听
	if b.listener != nil {
		b.listener.Close()
	}
	
	// 断开所有客户端
	for _, c := range b.clients {
		c.Disconnect()
	}
	
	// 停止批处理
	if b.batchProcessor != nil {
		b.batchProcessor.Stop()
	}
	
	// 停止系统主题
	if b.sysTopic != nil {
		b.sysTopic.Stop()
	}
	
	// 停止集群
	if b.cluster != nil {
		b.cluster.Stop()
	}
	
	// 关闭存储
	if b.storage != nil {
		b.storage.Close()
	}
}

// GetStats 获取统计信息
func (b *Broker) GetStats() *systopic.Stats {
	return b.sysTopic.GetStats()
}

// GetClients 获取客户端列表
func (b *Broker) GetClients() map[string]*client.Client {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	result := make(map[string]*client.Client)
	for k, v := range b.clients {
		result[k] = v
	}
	return result
}
