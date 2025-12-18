package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"mhmqtt/internal/protocol"

	"github.com/yyyear/YY"
	"go.etcd.io/bbolt"
)

// Storage 存储接口
type Storage interface {
	// 会话管理
	SaveSession(session *Session) error
	GetSession(clientID string) (*Session, error)
	DeleteSession(clientID string) error

	// 离线消息
	SaveOfflineMessage(clientID string, msg *protocol.Message) error
	GetOfflineMessages(clientID string) ([]*protocol.Message, error)
	DeleteOfflineMessage(clientID string, packetID uint16) error

	// Retained 消息
	SaveRetainedMessage(topic string, msg *protocol.Message) error
	GetRetainedMessage(topic string) (*protocol.Message, error)
	GetRetainedMessages() (map[string]*protocol.Message, error)
	DeleteRetainedMessage(topic string) error

	// 未过期消息
	SavePendingMessage(clientID string, packetID uint16, msg *protocol.Message) error
	GetPendingMessage(clientID string, packetID uint16) (*protocol.Message, error)
	DeletePendingMessage(clientID string, packetID uint16) error
	GetPendingMessages(clientID string) (map[uint16]*protocol.Message, error)

	Close() error
}

// Session 会话信息
type Session struct {
	ClientID      string
	CleanSession  bool
	Subscriptions map[string]byte // topic -> QoS
	WillMessage   *protocol.Message
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// BoltStorage BoltDB 存储实现
type BoltStorage struct {
	db *bbolt.DB
}

// NewBoltStorage 创建新的 BoltDB 存储
func NewBoltStorage(pathLog string) (*BoltStorage, error) {
	path := pathLog
	YY.Debug("opening boltdb storage at path: " + path)
	// 创建数据库文件
	f, err := os.Stat(path)
	if f != nil {
		YY.Debug("file info: "+f.Name(), err)
	}
	if err != nil {
		YY.Debug("stat file error: " + err.Error())
	}

	if os.IsNotExist(err) {
		YY.Debug("create data directory")
		os.MkdirAll("./data", 0755)
		file, err := os.Create(pathLog)
		if err != nil {
			return nil, err
		}
		file.Close()
	}

	db, err := bbolt.Open(path, 600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 创建 buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{
			"sessions",
			"offline_messages",
			"retained_messages",
			"pending_messages",
		}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &BoltStorage{db: db}, nil
}

// SaveSession 保存会话
func (s *BoltStorage) SaveSession(session *Session) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("sessions"))
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(session.ClientID), data)
	})
}

// GetSession 获取会话
func (s *BoltStorage) GetSession(clientID string) (*Session, error) {
	var session *Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("sessions"))
		data := bucket.Get([]byte(clientID))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &session)
	})
	return session, err
}

// DeleteSession 删除会话
func (s *BoltStorage) DeleteSession(clientID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("sessions"))
		return bucket.Delete([]byte(clientID))
	})
}

// SaveOfflineMessage 保存离线消息
func (s *BoltStorage) SaveOfflineMessage(clientID string, msg *protocol.Message) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("offline_messages"))
		key := fmt.Sprintf("%s:%d", clientID, msg.PacketID)
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), data)
	})
}

// GetOfflineMessages 获取离线消息
func (s *BoltStorage) GetOfflineMessages(clientID string) ([]*protocol.Message, error) {
	var messages []*protocol.Message
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("offline_messages"))
		cursor := bucket.Cursor()
		prefix := []byte(clientID + ":")

		for k, v := cursor.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = cursor.Next() {
			var msg protocol.Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}
			messages = append(messages, &msg)
		}
		return nil
	})
	return messages, err
}

// DeleteOfflineMessage 删除离线消息
func (s *BoltStorage) DeleteOfflineMessage(clientID string, packetID uint16) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("offline_messages"))
		key := fmt.Sprintf("%s:%d", clientID, packetID)
		return bucket.Delete([]byte(key))
	})
}

// SaveRetainedMessage 保存 Retained 消息
func (s *BoltStorage) SaveRetainedMessage(topic string, msg *protocol.Message) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("retained_messages"))
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(topic), data)
	})
}

// GetRetainedMessage 获取 Retained 消息
func (s *BoltStorage) GetRetainedMessage(topic string) (*protocol.Message, error) {
	var msg *protocol.Message
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("retained_messages"))
		data := bucket.Get([]byte(topic))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &msg)
	})
	return msg, err
}

// GetRetainedMessages 获取所有 Retained 消息
func (s *BoltStorage) GetRetainedMessages() (map[string]*protocol.Message, error) {
	messages := make(map[string]*protocol.Message)
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("retained_messages"))
		return bucket.ForEach(func(k, v []byte) error {
			var msg protocol.Message
			if err := json.Unmarshal(v, &msg); err != nil {
				return err
			}
			messages[string(k)] = &msg
			return nil
		})
	})
	return messages, err
}

// DeleteRetainedMessage 删除 Retained 消息
func (s *BoltStorage) DeleteRetainedMessage(topic string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("retained_messages"))
		return bucket.Delete([]byte(topic))
	})
}

// SavePendingMessage 保存待确认消息
func (s *BoltStorage) SavePendingMessage(clientID string, packetID uint16, msg *protocol.Message) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("pending_messages"))
		key := fmt.Sprintf("%s:%d", clientID, packetID)
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), data)
	})
}

// GetPendingMessage 获取待确认消息
func (s *BoltStorage) GetPendingMessage(clientID string, packetID uint16) (*protocol.Message, error) {
	var msg *protocol.Message
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("pending_messages"))
		key := fmt.Sprintf("%s:%d", clientID, packetID)
		data := bucket.Get([]byte(key))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &msg)
	})
	return msg, err
}

// DeletePendingMessage 删除待确认消息
func (s *BoltStorage) DeletePendingMessage(clientID string, packetID uint16) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("pending_messages"))
		key := fmt.Sprintf("%s:%d", clientID, packetID)
		return bucket.Delete([]byte(key))
	})
}

// GetPendingMessages 获取所有待确认消息
func (s *BoltStorage) GetPendingMessages(clientID string) (map[uint16]*protocol.Message, error) {
	messages := make(map[uint16]*protocol.Message)
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("pending_messages"))
		cursor := bucket.Cursor()
		prefix := []byte(clientID + ":")

		for k, v := cursor.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = cursor.Next() {
			var msg protocol.Message
			if err := json.Unmarshal(v, &msg); err != nil {
				continue
			}
			// 从 key 中提取 packetID
			keyStr := string(k)
			var packetID uint16
			_, _ = fmt.Sscanf(keyStr, clientID+":%d", &packetID)
			messages[packetID] = &msg
		}
		return nil
	})
	return messages, err
}

// Close 关闭存储
func (s *BoltStorage) Close() error {
	return s.db.Close()
}
