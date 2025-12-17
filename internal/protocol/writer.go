package protocol

import (
	"encoding/binary"
	"errors"
)

// PacketWriter 数据包写入器
type PacketWriter struct {
	buf []byte
}

func NewPacketWriter() *PacketWriter {
	return &PacketWriter{
		buf: make([]byte, 0, 256),
	}
}

// WritePacket 写入完整的数据包
func (w *PacketWriter) WritePacket(msgType byte, flags byte, payload []byte) []byte {
	w.buf = w.buf[:0]
	
	// 固定头
	fixedHeader := (msgType << 4) | (flags & 0x0F)
	w.buf = append(w.buf, fixedHeader)
	
	// 剩余长度
	remainingLen := len(payload)
	w.buf = append(w.buf, w.encodeRemainingLength(remainingLen)...)
	
	// 负载
	w.buf = append(w.buf, payload...)
	
	return w.buf
}

// encodeRemainingLength 编码剩余长度
func (w *PacketWriter) encodeRemainingLength(length int) []byte {
	var buf []byte
	x := length
	for {
		encodedByte := byte(x % 128)
		x /= 128
		if x > 0 {
			encodedByte |= 128
		}
		buf = append(buf, encodedByte)
		if x == 0 {
			break
		}
	}
	return buf
}

// WriteString 写入 UTF-8 字符串
func WriteString(buf []byte, s string) []byte {
	length := uint16(len(s))
	buf = append(buf, byte(length>>8), byte(length&0xFF))
	buf = append(buf, []byte(s)...)
	return buf
}

// WriteBytes 写入字节数组
func WriteBytes(buf []byte, data []byte) []byte {
	length := uint16(len(data))
	buf = append(buf, byte(length>>8), byte(length&0xFF))
	buf = append(buf, data...)
	return buf
}

// WriteUint16 写入 uint16
func WriteUint16(buf []byte, val uint16) []byte {
	return append(buf, byte(val>>8), byte(val&0xFF))
}

// WriteUint32 写入 uint32
func WriteUint32(buf []byte, val uint32) []byte {
	return append(buf, 
		byte(val>>24), 
		byte(val>>16), 
		byte(val>>8), 
		byte(val&0xFF))
}

// WriteVariableByteInteger 写入变长整数（v5.0）
func WriteVariableByteInteger(buf []byte, val uint32) []byte {
	for {
		encodedByte := byte(val % 128)
		val /= 128
		if val > 0 {
			encodedByte |= 128
		}
		buf = append(buf, encodedByte)
		if val == 0 {
			break
		}
	}
	return buf
}

// EncodeConnAck 编码 CONNACK 消息
func EncodeConnAck(version byte, returnCode byte, sessionPresent bool) []byte {
	writer := NewPacketWriter()
	payload := make([]byte, 0, 4)
	
	if version == Version50 {
		// v5.0
		var flags byte = 0
		if sessionPresent {
			flags |= 0x01
		}
		payload = append(payload, flags)
		payload = append(payload, returnCode)
		// 属性长度（简化，设为0）
		payload = append(payload, 0)
	} else {
		// v3.1.1
		var flags byte = 0
		if sessionPresent {
			flags |= 0x01
		}
		payload = append(payload, flags)
		payload = append(payload, returnCode)
	}
	
	return writer.WritePacket(CONNACK, 0, payload)
}

// EncodePublish 编码 PUBLISH 消息
func EncodePublish(msg *PublishMessage) []byte {
	writer := NewPacketWriter()
	payload := make([]byte, 0, 256)
	
	payload = WriteString(payload, msg.Topic)
	
	if msg.QoS > 0 {
		payload = WriteUint16(payload, msg.PacketID)
	}
	
	// v5.0 属性
	if msg.Properties != nil {
		propBuf := make([]byte, 0, 64)
		propBuf = WriteProperties(propBuf, msg.Properties)
		propLen := len(propBuf)
		payload = append(payload, WriteVariableByteInteger(make([]byte, 0, 4), uint32(propLen))...)
		payload = append(payload, propBuf...)
	}
	
	payload = append(payload, msg.Payload...)
	
	var flags byte = 0
	if msg.Retain {
		flags |= 0x01
	}
	flags |= (msg.QoS << 1)
	if msg.Dup {
		flags |= 0x08
	}
	
	return writer.WritePacket(PUBLISH, flags, payload)
}

// EncodePubAck 编码 PUBACK
func EncodePubAck(packetID uint16, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4), packetID)
	
	if version == Version50 {
		// v5.0 包含原因码和属性
		payload = append(payload, 0) // 原因码：成功
		payload = append(payload, 0) // 属性长度
	}
	
	return writer.WritePacket(PUBACK, 0, payload)
}

// EncodePubRec 编码 PUBREC
func EncodePubRec(packetID uint16, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4), packetID)
	
	if version == Version50 {
		payload = append(payload, 0) // 原因码
		payload = append(payload, 0) // 属性长度
	}
	
	return writer.WritePacket(PUBREC, 0, payload)
}

// EncodePubRel 编码 PUBREL
func EncodePubRel(packetID uint16, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4), packetID)
	
	if version == Version50 {
		payload = append(payload, 0) // 原因码
		payload = append(payload, 0) // 属性长度
	}
	
	return writer.WritePacket(PUBREL, 2, payload) // QoS 1
}

// EncodePubComp 编码 PUBCOMP
func EncodePubComp(packetID uint16, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4), packetID)
	
	if version == Version50 {
		payload = append(payload, 0) // 原因码
		payload = append(payload, 0) // 属性长度
	}
	
	return writer.WritePacket(PUBCOMP, 0, payload)
}

// EncodeSubAck 编码 SUBACK
func EncodeSubAck(packetID uint16, returnCodes []byte, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4+len(returnCodes)), packetID)
	
	if version == Version50 {
		// v5.0 属性长度
		payload = append(payload, 0)
	}
	
	payload = append(payload, returnCodes...)
	
	return writer.WritePacket(SUBACK, 0, payload)
}

// EncodeUnsubAck 编码 UNSUBACK
func EncodeUnsubAck(packetID uint16, returnCodes []byte, version byte) []byte {
	writer := NewPacketWriter()
	payload := WriteUint16(make([]byte, 0, 4+len(returnCodes)), packetID)
	
	if version == Version50 {
		// v5.0 属性长度
		payload = append(payload, 0)
		payload = append(payload, returnCodes...)
	}
	
	return writer.WritePacket(UNSUBACK, 0, payload)
}

// EncodePingResp 编码 PINGRESP
func EncodePingResp() []byte {
	writer := NewPacketWriter()
	return writer.WritePacket(PINGRESP, 0, nil)
}

// EncodeDisconnect 编码 DISCONNECT
func EncodeDisconnect(version byte, reasonCode byte) []byte {
	writer := NewPacketWriter()
	payload := make([]byte, 0, 4)
	
	if version == Version50 {
		payload = append(payload, reasonCode)
		payload = append(payload, 0) // 属性长度
	}
	
	return writer.WritePacket(DISCONNECT, 0, payload)
}

// WriteProperties 写入属性（v5.0）
func WriteProperties(buf []byte, props *Properties) []byte {
	if props == nil {
		return buf
	}
	
	if props.PayloadFormatIndicator != nil {
		buf = WriteVariableByteInteger(buf, 1)
		buf = append(buf, *props.PayloadFormatIndicator)
	}
	
	if props.MessageExpiryInterval != nil {
		buf = WriteVariableByteInteger(buf, 2)
		buf = WriteUint32(buf, *props.MessageExpiryInterval)
	}
	
	if props.ContentType != "" {
		buf = WriteVariableByteInteger(buf, 3)
		buf = WriteString(buf, props.ContentType)
	}
	
	if props.ResponseTopic != "" {
		buf = WriteVariableByteInteger(buf, 8)
		buf = WriteString(buf, props.ResponseTopic)
	}
	
	if len(props.CorrelationData) > 0 {
		buf = WriteVariableByteInteger(buf, 9)
		buf = WriteBytes(buf, props.CorrelationData)
	}
	
	if props.TopicAlias != nil {
		buf = WriteVariableByteInteger(buf, 11)
		buf = WriteUint16(buf, *props.TopicAlias)
	}
	
	for key, value := range props.UserProperties {
		buf = WriteVariableByteInteger(buf, 23)
		buf = WriteString(buf, key)
		buf = WriteString(buf, value)
	}
	
	return buf
}

