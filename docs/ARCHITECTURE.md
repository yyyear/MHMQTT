# MHMQTT 架构文档

## 项目结构

```
MHMQTT/
├── main.go                 # 程序入口
├── config.yaml            # 配置文件
├── go.mod                 # Go 模块定义
├── Makefile               # 构建脚本
├── internal/              # 内部包
│   ├── broker/            # Broker 核心逻辑
│   ├── client/            # 客户端连接处理
│   ├── cluster/           # 集群功能
│   ├── config/            # 配置管理
│   ├── httpapi/           # HTTP API
│   ├── message/           # 消息批处理
│   ├── pool/              # 连接池
│   ├── protocol/          # MQTT 协议解析
│   ├── storage/           # 存储层（BoltDB）
│   ├── systopic/          # 系统主题
│   └── topic/             # 主题管理
└── examples/              # 示例代码
```

## 核心组件

### 1. Protocol (协议层)
- **types.go**: MQTT 消息类型定义
- **reader.go**: 数据包读取和解析
- **writer.go**: 数据包编码和写入
- **parser.go**: 消息解析器

支持 MQTT v3.1, v3.1.1, v5.0 协议

### 2. Client (客户端管理)
- 处理客户端连接
- 管理客户端状态
- 处理 QoS 0/1/2 消息流
- Keep Alive 管理

### 3. Broker (Broker 核心)
- 连接管理
- 消息路由
- 会话管理
- 订阅管理

### 4. Storage (存储层)
- 会话持久化
- 离线消息存储
- Retained 消息存储
- 待确认消息存储

使用 BoltDB 作为存储引擎

### 5. Topic (主题管理)
- 主题订阅管理
- 通配符匹配（+ 和 #）
- 订阅者查找

### 6. Cluster (集群)
- 节点发现
- 消息转发
- 动态添加/移除节点

### 7. HTTP API
- RESTful API
- 状态查询
- 消息发布
- 集群管理

### 8. System Topic ($SYS)
- 系统统计信息
- 自动更新
- 标准 MQTT 系统主题

## 消息流程

### 发布消息流程
1. 客户端发送 PUBLISH
2. Broker 解析消息
3. 查找订阅者
4. 发送给在线订阅者
5. 保存离线消息（QoS > 0）
6. 保存 Retained 消息（如果设置）
7. 集群转发

### 订阅流程
1. 客户端发送 SUBSCRIBE
2. Broker 记录订阅
3. 发送 SUBACK
4. 发送 Retained 消息（如果有）
5. 更新会话

### QoS 流程

#### QoS 0
- 发布即忘记
- 不存储，不确认

#### QoS 1
- 至少一次交付
- 存储待确认消息
- PUBACK 确认

#### QoS 2
- 恰好一次交付
- PUBREC -> PUBREL -> PUBCOMP
- 完整的状态机

## 性能优化

### 连接池
- 复用连接
- 限制最大连接数
- 自动清理

### 批处理
- 批量处理消息
- 减少 I/O 操作
- 提高吞吐量

### 内存管理
- 消息缓存
- 及时释放资源
- 避免内存泄漏

## 集群架构

```
Node1 <---> Node2 <---> Node3
  |           |           |
  +-----------+-----------+
        消息转发
```

- 每个节点独立运行
- 消息在节点间转发
- 支持动态添加/移除节点

## 扩展性

- 水平扩展：通过集群
- 垂直扩展：优化单节点性能
- 插件系统：可扩展的架构

