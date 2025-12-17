package client

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"mhmqtt/internal/protocol"
	"mhmqtt/internal/storage"
)

// Client 客户端连接
type Client struct {
	ID           string
	Conn         net.Conn
	Version      byte
	KeepAlive    uint16
	CleanSession bool
	WillMessage  *protocol.Message
	
	// 状态
	mu              sync.RWMutex
	connected       bool
	lastPing        time.Time
	packetIDCounter uint16
	
	// 回调
	onDisconnect func(*Client)
	onPublish    func(*Client, *protocol.PublishMessage)
	onSubscribe  func(*Client, *protocol.SubscribeMessage)
	onUnsubscribe func(*Client, *protocol.UnsubscribeMessage)
	onPing        func(*Client)
	
	// 存储
	storage storage.Storage
	
	// QoS 2 状态
	pubRecMap map[uint16]bool // 已收到 PUBREC 的 PacketID
	pubRelMap map[uint16]bool // 已收到 PUBREL 的 PacketID
}

// NewClient 创建新客户端
func NewClient(conn net.Conn, storage storage.Storage) *Client {
	return &Client{
		Conn:      conn,
		connected: true,
		lastPing:  time.Now(),
		pubRecMap: make(map[uint16]bool),
		pubRelMap: make(map[uint16]bool),
		storage:   storage,
	}
}

// SetCallbacks 设置回调函数
func (c *Client) SetCallbacks(
	onDisconnect func(*Client),
	onPublish func(*Client, *protocol.PublishMessage),
	onSubscribe func(*Client, *protocol.SubscribeMessage),
	onUnsubscribe func(*Client, *protocol.UnsubscribeMessage),
	onPing func(*Client),
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = onDisconnect
	c.onPublish = onPublish
	c.onSubscribe = onSubscribe
	c.onUnsubscribe = onUnsubscribe
	c.onPing = onPing
}

// Start 启动客户端处理
func (c *Client) Start() error {
	// 设置读取超时
	if c.KeepAlive > 0 {
		timeout := time.Duration(c.KeepAlive) * time.Second * 3 / 2
		c.Conn.SetReadDeadline(time.Now().Add(timeout))
	}

	reader := protocol.NewPacketReader(c.Conn, 1024*1024) // 1MB max

	for {
		packet, err := reader.ReadPacket()
		if err != nil {
			if err == io.EOF {
				c.Disconnect()
				return nil
			}
			c.Disconnect()
			return err
		}

		// 更新读取超时
		if c.KeepAlive > 0 {
			timeout := time.Duration(c.KeepAlive) * time.Second * 3 / 2
			c.Conn.SetReadDeadline(time.Now().Add(timeout))
		}

		if err := c.handlePacket(packet); err != nil {
			c.Disconnect()
			return err
		}
	}
}

// handlePacket 处理数据包
func (c *Client) handlePacket(packet []byte) error {
	msg, err := protocol.ParsePacket(packet)
	if err != nil {
		return err
	}

	c.mu.RLock()
	onPublish := c.onPublish
	onSubscribe := c.onSubscribe
	onUnsubscribe := c.onUnsubscribe
	onPing := c.onPing
	c.mu.RUnlock()

	switch m := msg.(type) {
	case *protocol.ConnectMessage:
		return fmt.Errorf("重复的 CONNECT 消息")
	case *protocol.PublishMessage:
		if onPublish != nil {
			onPublish(c, m)
		}
		return c.handlePublish(m)
	case *protocol.PubAckMessage:
		return c.handlePubAck(m)
	case *protocol.PubRecMessage:
		return c.handlePubRec(m)
	case *protocol.PubRelMessage:
		return c.handlePubRel(m)
	case *protocol.PubCompMessage:
		return c.handlePubComp(m)
	case *protocol.SubscribeMessage:
		if onSubscribe != nil {
			onSubscribe(c, m)
		}
		return nil // 由回调处理
	case *protocol.UnsubscribeMessage:
		if onUnsubscribe != nil {
			onUnsubscribe(c, m)
		}
		return nil // 由回调处理
	case *protocol.PingReqMessage:
		if onPing != nil {
			onPing(c)
		}
		return c.handlePing()
	case *protocol.DisconnectMessage:
		c.Disconnect()
		return nil
	default:
		return fmt.Errorf("未知消息类型")
	}
}

// handlePublish 处理 PUBLISH
func (c *Client) handlePublish(msg *protocol.PublishMessage) error {
	switch msg.QoS {
	case 0:
		// QoS 0 不需要确认
		return nil
	case 1:
		// QoS 1 发送 PUBACK
		return c.SendPubAck(msg.PacketID)
	case 2:
		// QoS 2 发送 PUBREC
		return c.SendPubRec(msg.PacketID)
	default:
		return fmt.Errorf("无效的 QoS: %d", msg.QoS)
	}
}

// handlePubAck 处理 PUBACK
func (c *Client) handlePubAck(msg *protocol.PubAckMessage) error {
	// 删除待确认消息
	return c.storage.DeletePendingMessage(c.ID, msg.PacketID)
}

// handlePubRec 处理 PUBREC
func (c *Client) handlePubRec(msg *protocol.PubRecMessage) error {
	c.mu.Lock()
	c.pubRecMap[msg.PacketID] = true
	c.mu.Unlock()
	
	// 发送 PUBREL
	return c.SendPubRel(msg.PacketID)
}

// handlePubRel 处理 PUBREL
func (c *Client) handlePubRel(msg *protocol.PubRelMessage) error {
	c.mu.Lock()
	c.pubRelMap[msg.PacketID] = true
	c.mu.Unlock()
	
	// 发送 PUBCOMP
	return c.SendPubComp(msg.PacketID)
}

// handlePubComp 处理 PUBCOMP
func (c *Client) handlePubComp(msg *protocol.PubCompMessage) error {
	// 删除待确认消息
	return c.storage.DeletePendingMessage(c.ID, msg.PacketID)
}

// handlePing 处理 PINGREQ
func (c *Client) handlePing() error {
	c.mu.Lock()
	c.lastPing = time.Now()
	c.mu.Unlock()
	return c.SendPingResp()
}

// SendConnAck 发送 CONNACK
func (c *Client) SendConnAck(returnCode byte, sessionPresent bool) error {
	data := protocol.EncodeConnAck(c.Version, returnCode, sessionPresent)
	_, err := c.Conn.Write(data)
	return err
}

// SendPublish 发送 PUBLISH
func (c *Client) SendPublish(msg *protocol.PublishMessage) error {
	data := protocol.EncodePublish(msg)
	_, err := c.Conn.Write(data)
	return err
}

// SendPubAck 发送 PUBACK
func (c *Client) SendPubAck(packetID uint16) error {
	data := protocol.EncodePubAck(packetID, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendPubRec 发送 PUBREC
func (c *Client) SendPubRec(packetID uint16) error {
	data := protocol.EncodePubRec(packetID, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendPubRel 发送 PUBREL
func (c *Client) SendPubRel(packetID uint16) error {
	data := protocol.EncodePubRel(packetID, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendPubComp 发送 PUBCOMP
func (c *Client) SendPubComp(packetID uint16) error {
	data := protocol.EncodePubComp(packetID, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendSubAck 发送 SUBACK
func (c *Client) SendSubAck(packetID uint16, returnCodes []byte) error {
	data := protocol.EncodeSubAck(packetID, returnCodes, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendUnsubAck 发送 UNSUBACK
func (c *Client) SendUnsubAck(packetID uint16) error {
	returnCodes := []byte{0} // 简化处理
	data := protocol.EncodeUnsubAck(packetID, returnCodes, c.Version)
	_, err := c.Conn.Write(data)
	return err
}

// SendPingResp 发送 PINGRESP
func (c *Client) SendPingResp() error {
	data := protocol.EncodePingResp()
	_, err := c.Conn.Write(data)
	return err
}

// SendDisconnect 发送 DISCONNECT
func (c *Client) SendDisconnect(reasonCode byte) error {
	data := protocol.EncodeDisconnect(c.Version, reasonCode)
	_, err := c.Conn.Write(data)
	return err
}

// NextPacketID 获取下一个 Packet ID
func (c *Client) NextPacketID() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.packetIDCounter++
	if c.packetIDCounter == 0 {
		c.packetIDCounter = 1
	}
	return c.packetIDCounter
}

// Disconnect 断开连接
func (c *Client) Disconnect() {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return
	}
	c.connected = false
	onDisconnect := c.onDisconnect
	c.mu.Unlock()

	if c.Conn != nil {
		c.Conn.Close()
	}

	if onDisconnect != nil {
		onDisconnect(c)
	}
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetLastPing 获取最后 ping 时间
func (c *Client) GetLastPing() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastPing
}

