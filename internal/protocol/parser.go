package protocol

import (
	"encoding/binary"
	"errors"
)

// ParsePacket 解析数据包
func ParsePacket(data []byte) (interface{}, error) {
	if len(data) < 2 {
		return nil, ErrInvalidPacket
	}

	msgType := (data[0] >> 4) & 0x0F
	flags := data[0] & 0x0F

	offset := 1
	remainingLen, n, err := readRemainingLength(data, offset)
	if err != nil {
		return nil, err
	}
	offset += n

	if offset+remainingLen > len(data) {
		return nil, ErrInvalidPacket
	}

	payload := data[offset : offset+remainingLen]

	switch msgType {
	case CONNECT:
		return ParseConnect(data)
	case PUBLISH:
		return ParsePublish(data, flags)
	case PUBACK:
		return ParsePubAck(payload)
	case PUBREC:
		return ParsePubRec(payload)
	case PUBREL:
		return ParsePubRel(payload)
	case PUBCOMP:
		return ParsePubComp(payload)
	case SUBSCRIBE:
		return ParseSubscribe(data)
	case UNSUBSCRIBE:
		return ParseUnsubscribe(data)
	case PINGREQ:
		return &PingReqMessage{}, nil
	case DISCONNECT:
		return ParseDisconnect(payload)
	default:
		return nil, errors.New("不支持的消息类型")
	}
}

// ParsePublish 解析 PUBLISH 消息
func ParsePublish(data []byte, flags byte) (*PublishMessage, error) {
	offset := 1
	_, n, err := readRemainingLength(data, offset)
	if err != nil {
		return nil, err
	}
	offset += n

	msg := &PublishMessage{}
	msg.Dup = (flags & 0x08) != 0
	msg.QoS = (flags >> 1) & 0x03
	msg.Retain = (flags & 0x01) != 0

	// 读取主题
	msg.Topic, offset, err = ReadString(data, offset)
	if err != nil {
		return nil, err
	}

	// 读取 Packet ID（QoS > 0）
	if msg.QoS > 0 {
		msg.PacketID, offset, err = ReadUint16(data, offset)
		if err != nil {
			return nil, err
		}
	}

	// v5.0 属性（简化处理，假设没有属性）
	// 实际实现中需要根据协议版本判断

	// 剩余的是 payload
	msg.Payload = data[offset:]

	return msg, nil
}

// ParsePubAck 解析 PUBACK
func ParsePubAck(payload []byte) (*PubAckMessage, error) {
	if len(payload) < 2 {
		return nil, ErrInvalidPacket
	}
	return &PubAckMessage{
		PacketID: binary.BigEndian.Uint16(payload),
	}, nil
}

// ParsePubRec 解析 PUBREC
func ParsePubRec(payload []byte) (*PubRecMessage, error) {
	if len(payload) < 2 {
		return nil, ErrInvalidPacket
	}
	return &PubRecMessage{
		PacketID: binary.BigEndian.Uint16(payload),
	}, nil
}

// ParsePubRel 解析 PUBREL
func ParsePubRel(payload []byte) (*PubRelMessage, error) {
	if len(payload) < 2 {
		return nil, ErrInvalidPacket
	}
	return &PubRelMessage{
		PacketID: binary.BigEndian.Uint16(payload),
	}, nil
}

// ParsePubComp 解析 PUBCOMP
func ParsePubComp(payload []byte) (*PubCompMessage, error) {
	if len(payload) < 2 {
		return nil, ErrInvalidPacket
	}
	return &PubCompMessage{
		PacketID: binary.BigEndian.Uint16(payload),
	}, nil
}

// ParseSubscribe 解析 SUBSCRIBE 消息
func ParseSubscribe(data []byte) (*SubscribeMessage, error) {
	offset := 1
	_, n, err := readRemainingLength(data, offset)
	if err != nil {
		return nil, err
	}
	offset += n

	msg := &SubscribeMessage{}

	// 读取 Packet ID
	msg.PacketID, offset, err = ReadUint16(data, offset)
	if err != nil {
		return nil, err
	}

	// v5.0 属性（简化处理）
	// 实际实现中需要根据协议版本判断

	// 读取订阅列表
	msg.Topics = []TopicSubscription{}
	for offset < len(data) {
		topic, newOffset, err := ReadString(data, offset)
		if err != nil {
			break
		}
		offset = newOffset

		if offset >= len(data) {
			return nil, ErrInvalidPacket
		}

		options := data[offset]
		offset++

		sub := TopicSubscription{
			Topic: topic,
			QoS:   options & 0x03,
		}

		// v5.0 选项
		if (options & 0x04) != 0 {
			sub.NoLocal = true
		}
		if (options & 0x08) != 0 {
			sub.RetainAsPublished = true
		}
		sub.RetainHandling = (options >> 4) & 0x03

		msg.Topics = append(msg.Topics, sub)
	}

	return msg, nil
}

// ParseUnsubscribe 解析 UNSUBSCRIBE 消息
func ParseUnsubscribe(data []byte) (*UnsubscribeMessage, error) {
	offset := 1
	_, n, err := readRemainingLength(data, offset)
	if err != nil {
		return nil, err
	}
	offset += n

	msg := &UnsubscribeMessage{}

	// 读取 Packet ID
	msg.PacketID, offset, err = ReadUint16(data, offset)
	if err != nil {
		return nil, err
	}

	// 读取主题列表
	msg.Topics = []string{}
	for offset < len(data) {
		topic, newOffset, err := ReadString(data, offset)
		if err != nil {
			break
		}
		offset = newOffset
		msg.Topics = append(msg.Topics, topic)
	}

	return msg, nil
}

// ParseDisconnect 解析 DISCONNECT 消息
func ParseDisconnect(payload []byte) (*DisconnectMessage, error) {
	msg := &DisconnectMessage{}
	if len(payload) > 0 {
		msg.ReasonCode = payload[0]
	}
	return msg, nil
}

// 辅助消息类型
type PubAckMessage struct {
	PacketID uint16
}

type PubRecMessage struct {
	PacketID uint16
}

type PubRelMessage struct {
	PacketID uint16
}

type PubCompMessage struct {
	PacketID uint16
}

type PingReqMessage struct{}

type UnsubscribeMessage struct {
	PacketID uint16
	Topics   []string
}

type DisconnectMessage struct {
	ReasonCode byte
}

