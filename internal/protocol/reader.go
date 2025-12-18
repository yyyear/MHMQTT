package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/yyyear/YY"
)

var (
	ErrInvalidPacket  = errors.New("无效的数据包")
	ErrPacketTooLarge = errors.New("数据包过大")
)

// PacketReader 数据包读取器
type PacketReader struct {
	reader  io.Reader
	maxSize int
}

func NewPacketReader(reader io.Reader, maxSize int) *PacketReader {
	return &PacketReader{
		reader:  reader,
		maxSize: maxSize,
	}
}

// ReadPacket 读取一个完整的 MQTT 数据包
func (r *PacketReader) ReadPacket() ([]byte, error) {
	// 读取固定头
	fixedHeader := make([]byte, 1)
	if _, err := io.ReadFull(r.reader, fixedHeader); err != nil {
		return nil, err
	}

	// 读取剩余长度
	remainingLength, n, err := r.readRemainingLength()
	if err != nil {
		return nil, err
	}

	if remainingLength > r.maxSize {
		return nil, ErrPacketTooLarge
	}

	// 读取剩余数据
	packet := make([]byte, 1+n+remainingLength)
	packet[0] = fixedHeader[0]
	copy(packet[1:1+n], r.encodeRemainingLength(remainingLength))

	if remainingLength > 0 {
		if _, err := io.ReadFull(r.reader, packet[1+n:]); err != nil {
			return nil, err
		}
	}

	return packet, nil
}

// readRemainingLength 读取剩余长度（变长编码）
func (r *PacketReader) readRemainingLength() (int, int, error) {
	multiplier := 1
	value := 0
	bytesRead := 0

	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(r.reader, b); err != nil {
			return 0, 0, err
		}
		bytesRead++

		value += int(b[0]&127) * multiplier
		multiplier *= 128

		if multiplier > 128*128*128 {
			return 0, 0, ErrInvalidPacket
		}

		if (b[0] & 128) == 0 {
			break
		}
	}

	return value, bytesRead, nil
}

// encodeRemainingLength 编码剩余长度
func (r *PacketReader) encodeRemainingLength(length int) []byte {
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

// ReadString 读取 UTF-8 字符串
func ReadString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", 0, ErrInvalidPacket
	}

	length := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if offset+length > len(data) {
		return "", 0, ErrInvalidPacket
	}

	str := string(data[offset : offset+length])
	offset += length

	return str, offset, nil
}

// ReadBytes 读取字节数组
func ReadBytes(data []byte, offset int) ([]byte, int, error) {
	if offset+2 > len(data) {
		return nil, 0, ErrInvalidPacket
	}

	length := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if offset+length > len(data) {
		return nil, 0, ErrInvalidPacket
	}

	bytes := make([]byte, length)
	copy(bytes, data[offset:offset+length])
	offset += length

	return bytes, offset, nil
}

// ReadByte 读取单个字节
func ReadByte(data []byte, offset int) (byte, int, error) {
	if offset >= len(data) {
		return 0, 0, ErrInvalidPacket
	}
	return data[offset], offset + 1, nil
}

// ReadUint16 读取 uint16
func ReadUint16(data []byte, offset int) (uint16, int, error) {
	if offset+2 > len(data) {
		return 0, 0, ErrInvalidPacket
	}
	return binary.BigEndian.Uint16(data[offset:]), offset + 2, nil
}

// ReadUint32 读取 uint32
func ReadUint32(data []byte, offset int) (uint32, int, error) {
	if offset+4 > len(data) {
		return 0, 0, ErrInvalidPacket
	}
	return binary.BigEndian.Uint32(data[offset:]), offset + 4, nil
}

// ParseConnect 解析 CONNECT 消息
func ParseConnect(data []byte) (*ConnectMessage, error) {
	if len(data) < 2 {
		return nil, ErrInvalidPacket
	}

	msgType := (data[0] >> 4) & 0x0F
	if msgType != CONNECT {
		return nil, fmt.Errorf("不是 CONNECT 消息")
	}
	YY.Debug("解析 CONNECT 消息:", []byte(data))
	offset := 1
	// 跳过剩余长度
	_, n, err := readRemainingLength(data, offset)
	if err != nil {
		YY.Error("读取剩余长度失败:", err)
		return nil, err
	}
	offset += n

	msg := &ConnectMessage{}

	// 读取协议名
	msg.ProtocolName, offset, err = ReadString(data, offset)
	if err != nil {
		YY.Error("读取协议名失败:", err)
		return nil, err
	}

	// 读取协议版本
	if offset >= len(data) {
		YY.Error("读取协议版本失败: 数据不足")
		return nil, ErrInvalidPacket
	}
	msg.ProtocolVersion = data[offset]
	offset++

	// 读取连接标志
	if offset >= len(data) {
		YY.Error("读取连接标志失败: 数据不足")
		return nil, ErrInvalidPacket
	}
	flags := data[offset]
	msg.CleanSession = (flags & 0x02) != 0
	msg.WillFlag = (flags & 0x04) != 0
	msg.WillQoS = (flags >> 3) & 0x03
	msg.WillRetain = (flags & 0x20) != 0
	msg.PasswordFlag = (flags & 0x40) != 0
	msg.UsernameFlag = (flags & 0x80) != 0
	offset++

	// 读取 Keep Alive
	msg.KeepAlive, offset, err = ReadUint16(data, offset)
	if err != nil {
		YY.Error("读取 Keep Alive 失败:", err)
		return nil, err
	}

	// v5.0 属性
	if msg.ProtocolVersion == Version50 {
		// 跳过属性长度
		propLen, n, err := readVariableByteInteger(data, offset)
		if err != nil {
			YY.Error("读取属性长度失败:", err)
			return nil, err
		}
		offset += n
		if propLen > 0 {
			msg.Properties, offset, err = ReadProperties(data, offset, int(propLen))
			if err != nil {
				YY.Error("读取属性失败:", err)
				return nil, err
			}
		}
	}

	// 读取 Client ID
	msg.ClientID, offset, err = ReadString(data, offset)
	if err != nil {
		YY.Error("读取 Client ID 失败:", err)
		return nil, err
	}

	// 读取 Will 消息
	if msg.WillFlag {
		msg.WillTopic, offset, err = ReadString(data, offset)
		if err != nil {
			YY.Error("读取 Will 主题失败:", err)
			return nil, err
		}
		msg.WillMessage, offset, err = ReadBytes(data, offset)
		if err != nil {
			YY.Error("读取 Will 消息失败:", err)
			return nil, err
		}
	}

	// 读取用户名
	if msg.UsernameFlag {
		msg.Username, offset, err = ReadString(data, offset)
		YY.Debug("读取用户名:", msg.Username)
		if err != nil {
			YY.Error("读取用户名失败:", err)
			return nil, err
		}
	}

	// 读取密码
	if msg.PasswordFlag {
		msg.Password, offset, err = ReadBytes(data, offset)
		YY.Debug("读取密码:", string(msg.Password))
		if err != nil {
			YY.Error("读取密码失败:", err)
			return nil, err
		}
	}

	return msg, nil
}

// readRemainingLength 辅助函数
func readRemainingLength(data []byte, offset int) (int, int, error) {
	multiplier := 1
	value := 0
	bytesRead := 0

	for {
		if offset >= len(data) {
			return 0, 0, ErrInvalidPacket
		}

		b := data[offset]
		offset++
		bytesRead++

		value += int(b&127) * multiplier
		multiplier *= 128

		if multiplier > 128*128*128 {
			return 0, 0, ErrInvalidPacket
		}

		if (b & 128) == 0 {
			break
		}
	}

	return value, bytesRead, nil
}

// readVariableByteInteger 读取变长整数（v5.0）
func readVariableByteInteger(data []byte, offset int) (uint32, int, error) {
	multiplier := uint32(1)
	value := uint32(0)
	bytesRead := 0

	for {
		if offset >= len(data) {
			return 0, 0, ErrInvalidPacket
		}

		b := data[offset]
		offset++
		bytesRead++

		value += uint32(b&127) * multiplier
		multiplier *= 128

		if multiplier > 128*128*128*128 {
			return 0, 0, ErrInvalidPacket
		}

		if (b & 128) == 0 {
			break
		}
	}

	return value, bytesRead, nil
}

// ReadProperties 读取属性（v5.0）
func ReadProperties(data []byte, offset int, maxLen int) (*Properties, int, error) {
	props := &Properties{
		UserProperties: make(map[string]string),
	}
	startOffset := offset
	endOffset := offset + maxLen

	for offset < endOffset {
		if offset >= len(data) {
			break
		}
		propID, n, err := readVariableByteInteger(data, offset)
		if err != nil {
			return nil, startOffset, err
		}
		offset += n
		switch propID {
		case 1: // Payload Format Indicator
			if offset >= len(data) {
				return nil, startOffset, ErrInvalidPacket
			}
			val := data[offset]
			props.PayloadFormatIndicator = &val
			offset++
		case 2: // Message Expiry Interval
			val, newOffset, err := ReadUint32(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			props.MessageExpiryInterval = &val
			offset = newOffset
		case 17: // Session Expiry Interval (CONNECT 专用，解析后暂存到属性中，供上层处理)
			val, newOffset, err := ReadUint32(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			props.SessionExpiryInterval = &val
			offset = newOffset
		case 3: // Content Type
			props.ContentType, offset, err = ReadString(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
		case 8: // Response Topic
			props.ResponseTopic, offset, err = ReadString(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
		case 9: // Correlation Data
			props.CorrelationData, offset, err = ReadBytes(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
		case 11: // Topic Alias
			val16, newOffset, err := ReadUint16(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			props.TopicAlias = &val16
			offset = newOffset
		case 23: // User Property
			key, newOffset, err := ReadString(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			offset = newOffset
			value, newOffset, err := ReadString(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			props.UserProperties[key] = value
			offset = newOffset
		case 26: // Subscription Identifier
			// 简化处理，只读取第一个
			val, n, err := readVariableByteInteger(data, offset)
			if err != nil {
				return nil, startOffset, err
			}
			props.SubscriptionIdentifier = append(props.SubscriptionIdentifier, val)
			offset += n
		default:
			// 跳过未知属性
			return nil, startOffset, fmt.Errorf("未知属性 ID: %d", propID)
		}
	}

	return props, offset, nil
}
