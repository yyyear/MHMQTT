package protocol

import "sync"

// MQTT 协议版本
const (
	Version31  byte = 3
	Version311 byte = 4
	Version50  byte = 5
)

// 对象池 - 用于复用消息对象，减少 GC 压力
var (
	publishMessagePool = sync.Pool{
		New: func() interface{} {
			return &PublishMessage{}
		},
	}

	messagePool = sync.Pool{
		New: func() interface{} {
			return &Message{}
		},
	}

	// 字节切片池 - 用于复用消息 payload
	byteSlicePool = sync.Pool{
		New: func() interface{} {
			// 预分配 1KB 的切片
			buf := make([]byte, 0, 1024)
			return &buf
		},
	}
)

// AcquirePublishMessage 从池中获取 PublishMessage
func AcquirePublishMessage() *PublishMessage {
	return publishMessagePool.Get().(*PublishMessage)
}

// ReleasePublishMessage 将 PublishMessage 放回池中
func ReleasePublishMessage(msg *PublishMessage) {
	if msg == nil {
		return
	}
	// 重置字段
	msg.Topic = ""
	msg.Payload = nil
	msg.QoS = 0
	msg.Retain = false
	msg.PacketID = 0
	msg.Dup = false
	msg.Properties = nil
	publishMessagePool.Put(msg)
}

// AcquireMessage 从池中获取 Message
func AcquireMessage() *Message {
	return messagePool.Get().(*Message)
}

// ReleaseMessage 将 Message 放回池中
func ReleaseMessage(msg *Message) {
	if msg == nil {
		return
	}
	// 重置字段
	msg.Topic = ""
	msg.Payload = nil
	msg.QoS = 0
	msg.Retain = false
	msg.PacketID = 0
	msg.Dup = false
	msg.Properties = nil
	messagePool.Put(msg)
}

// AcquireByteSlice 从池中获取字节切片
func AcquireByteSlice(size int) []byte {
	bp := byteSlicePool.Get().(*[]byte)
	buf := *bp
	if cap(buf) >= size {
		return buf[:size]
	}
	// 如果容量不够，创建新的更大的切片
	byteSlicePool.Put(bp)
	return make([]byte, size)
}

// ReleaseByteSlice 将字节切片放回池中
func ReleaseByteSlice(buf []byte) {
	if buf == nil || cap(buf) > 64*1024 { // 不缓存超过 64KB 的切片
		return
	}
	buf = buf[:0]
	byteSlicePool.Put(&buf)
}

// 消息类型
const (
	CONNECT     byte = 1
	CONNACK     byte = 2
	PUBLISH     byte = 3
	PUBACK      byte = 4
	PUBREC      byte = 5
	PUBREL      byte = 6
	PUBCOMP     byte = 7
	SUBSCRIBE   byte = 8
	SUBACK      byte = 9
	UNSUBSCRIBE byte = 10
	UNSUBACK    byte = 11
	PINGREQ     byte = 12
	PINGRESP    byte = 13
	DISCONNECT  byte = 14
	AUTH        byte = 15
)

// QoS 级别
const (
	QoS0 byte = 0
	QoS1 byte = 1
	QoS2 byte = 2
)

// 连接返回码 (v3.1.1)
const (
	ConnAckAccepted             byte = 0
	ConnAckUnacceptableProtocol byte = 1
	ConnAckIdentifierRejected   byte = 2
	ConnAckServerUnavailable    byte = 3
	ConnAckBadUsernamePassword  byte = 4
	ConnAckNotAuthorized        byte = 5
)

// 连接返回码 (v5.0)
const (
	ConnAckV5Success                     byte = 0
	ConnAckV5UnspecifiedError            byte = 128
	ConnAckV5MalformedPacket             byte = 129
	ConnAckV5ProtocolError               byte = 130
	ConnAckV5ImplementationSpecificError byte = 131
	ConnAckV5UnsupportedProtocolVersion  byte = 132
	ConnAckV5ClientIdentifierNotValid    byte = 133
	ConnAckV5BadUsernamePassword         byte = 134
	ConnAckV5NotAuthorized               byte = 135
	ConnAckV5ServerUnavailable           byte = 136
	ConnAckV5ServerBusy                  byte = 137
	ConnAckV5Banned                      byte = 138
	ConnAckV5BadAuthenticationMethod     byte = 140
	ConnAckV5TopicNameInvalid            byte = 144
	ConnAckV5PacketTooLarge              byte = 149
	ConnAckV5QuotaExceeded               byte = 151
	ConnAckV5PayloadFormatInvalid        byte = 153
	ConnAckV5RetainNotSupported          byte = 154
	ConnAckV5QoSNotSupported             byte = 155
	ConnAckV5UseAnotherServer            byte = 156
	ConnAckV5ServerMoved                 byte = 157
	ConnAckV5ConnectionRateExceeded      byte = 159
)

// 消息结构
type Message struct {
	Topic      string
	Payload    []byte
	QoS        byte
	Retain     bool
	PacketID   uint16
	Dup        bool
	Properties *Properties // v5.0 属性
}

// MQTT v5.0 属性
type Properties struct {
	PayloadFormatIndicator *byte
	MessageExpiryInterval  *uint32
	SessionExpiryInterval  *uint32
	ContentType            string
	ResponseTopic          string
	CorrelationData        []byte
	UserProperties         map[string]string
	SubscriptionIdentifier []uint32
	TopicAlias             *uint16
}

// Connect 消息
type ConnectMessage struct {
	ProtocolName    string
	ProtocolVersion byte
	CleanSession    bool
	WillFlag        bool
	WillQoS         byte
	WillRetain      bool
	PasswordFlag    bool
	UsernameFlag    bool
	KeepAlive       uint16
	ClientID        string
	WillTopic       string
	WillMessage     []byte
	Username        string
	Password        []byte
	Properties      *Properties // v5.0
}

// Subscribe 消息
type SubscribeMessage struct {
	PacketID   uint16
	Topics     []TopicSubscription
	Properties *Properties // v5.0
}

// TopicSubscription 订阅主题
type TopicSubscription struct {
	Topic             string
	QoS               byte
	NoLocal           bool // v5.0
	RetainAsPublished bool // v5.0
	RetainHandling    byte // v5.0
}

// Publish 消息
type PublishMessage struct {
	Topic      string
	Payload    []byte
	QoS        byte
	Retain     bool
	PacketID   uint16
	Dup        bool
	Properties *Properties // v5.0
}
