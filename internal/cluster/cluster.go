package cluster

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"mhmqtt/internal/config"
	"mhmqtt/internal/protocol"
)

// Cluster 集群管理器
type Cluster struct {
	config  config.ClusterConfig
	broker  interface{} // *broker.Broker 避免循环依赖
	mu      sync.RWMutex
	nodes   map[string]*Node
	running bool
	listener net.Listener
}

// Node 集群节点
type Node struct {
	ID      string
	Address string
	Conn    net.Conn
	mu      sync.Mutex
}

// NewCluster 创建集群
func NewCluster(cfg config.ClusterConfig, broker interface{}) *Cluster {
	return &Cluster{
		config: cfg,
		broker: broker,
		nodes:  make(map[string]*Node),
	}
}

// Start 启动集群
func (c *Cluster) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("集群已在运行")
	}

	// 启动集群监听
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.config.ClusterPort))
	if err != nil {
		return fmt.Errorf("监听集群端口失败: %w", err)
	}
	c.listener = listener

	c.running = true

	// 启动接受连接
	go c.acceptConnections()

	// 连接到其他节点
	go c.connectToNodes()

	return nil
}

// acceptConnections 接受集群连接
func (c *Cluster) acceptConnections() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			if c.isRunning() {
				log.Printf("接受集群连接失败: %v", err)
			}
			return
		}

		go c.handleNodeConnection(conn)
	}
}

// handleNodeConnection 处理节点连接
func (c *Cluster) handleNodeConnection(conn net.Conn) {
	defer conn.Close()

	// 读取节点信息
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	nodeID := string(buf[:n])

	c.mu.Lock()
	c.nodes[nodeID] = &Node{
		ID:      nodeID,
		Address: conn.RemoteAddr().String(),
		Conn:    conn,
	}
	c.mu.Unlock()

	log.Printf("节点 %s 已连接", nodeID)

	// 保持连接
	c.keepAlive(conn)
}

// connectToNodes 连接到其他节点
func (c *Cluster) connectToNodes() {
	for _, nodeAddr := range c.config.Nodes {
		go func(addr string) {
			for {
				if !c.isRunning() {
					return
				}

				conn, err := net.Dial("tcp", addr)
				if err != nil {
					time.Sleep(5 * time.Second)
					continue
				}

				// 发送节点 ID
				_, err = conn.Write([]byte(c.config.NodeID))
				if err != nil {
					conn.Close()
					time.Sleep(5 * time.Second)
					continue
				}

				c.mu.Lock()
				c.nodes[addr] = &Node{
					ID:      addr,
					Address: addr,
					Conn:    conn,
				}
				c.mu.Unlock()

				log.Printf("已连接到节点 %s", addr)

				// 保持连接
				c.keepAlive(conn)
			}
		}(nodeAddr)
	}
}

// keepAlive 保持连接
func (c *Cluster) keepAlive(conn net.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			// 发送心跳
			_, err := conn.Write([]byte{0})
			if err != nil {
				return
			}
		default:
			// 检查连接
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}
}

// ForwardMessage 转发消息到其他节点
func (c *Cluster) ForwardMessage(msg *protocol.Message) {
	c.mu.RLock()
	nodes := make([]*Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	c.mu.RUnlock()

	// 序列化消息（简化处理）
	data := c.serializeMessage(msg)

	for _, node := range nodes {
		node.mu.Lock()
		if node.Conn != nil {
			_, err := node.Conn.Write(data)
			if err != nil {
				node.Conn.Close()
				node.Conn = nil
			}
		}
		node.mu.Unlock()
	}
}

// serializeMessage 序列化消息
func (c *Cluster) serializeMessage(msg *protocol.Message) []byte {
	// 简化实现，实际应该使用 protobuf 或其他序列化格式
	pubMsg := &protocol.PublishMessage{
		Topic:   msg.Topic,
		Payload: msg.Payload,
		QoS:     msg.QoS,
		Retain:  msg.Retain,
	}
	return protocol.EncodePublish(pubMsg)
}

// AddNode 动态添加节点
func (c *Cluster) AddNode(nodeID, address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[nodeID]; exists {
		return fmt.Errorf("节点 %s 已存在", nodeID)
	}

	// 连接到新节点
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("连接节点失败: %w", err)
	}

	// 发送节点 ID
	_, err = conn.Write([]byte(c.config.NodeID))
	if err != nil {
		conn.Close()
		return err
	}

	c.nodes[nodeID] = &Node{
		ID:      nodeID,
		Address: address,
		Conn:    conn,
	}

	log.Printf("已添加节点 %s (%s)", nodeID, address)

	// 保持连接
	go c.keepAlive(conn)

	return nil
}

// RemoveNode 移除节点
func (c *Cluster) RemoveNode(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.nodes[nodeID]
	if !exists {
		return fmt.Errorf("节点 %s 不存在", nodeID)
	}

	if node.Conn != nil {
		node.Conn.Close()
	}

	delete(c.nodes, nodeID)

	log.Printf("已移除节点 %s", nodeID)

	return nil
}

// isRunning 检查是否运行中
func (c *Cluster) isRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// Stop 停止集群
func (c *Cluster) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false

	// 关闭监听
	if c.listener != nil {
		c.listener.Close()
	}

	// 关闭所有连接
	for _, node := range c.nodes {
		if node.Conn != nil {
			node.Conn.Close()
		}
	}

	c.nodes = make(map[string]*Node)
}

