# 快速开始指南

## 安装

### 前置要求
- Go 1.21 或更高版本
- 至少 100MB 可用磁盘空间（用于数据存储）

### 构建

```bash
# 下载依赖
go mod download

# 构建
make build
# 或
go build -o bin/mhmqtt main.go
```

## 配置

编辑 `config.yaml` 文件：

```yaml
broker:
  address: ":1883"        # MQTT 端口
  ws_address: ":8083"     # WebSocket 端口
  http_address: ":8080"    # HTTP API 端口
  keep_alive: 300         # Keep Alive 时间（秒）
  max_clients: 10000      # 最大客户端数
  data_path: "./data"     # 数据存储路径
```

## 运行

```bash
# 直接运行
make run
# 或
go run main.go

# 使用自定义配置
go run main.go -config=config.yaml
```

## 测试连接

### 使用 mosquitto 客户端

```bash
# 订阅
mosquitto_sub -h localhost -p 1883 -t "test/topic"

# 发布
mosquitto_pub -h localhost -p 1883 -t "test/topic" -m "Hello MQTT"
```

### 使用 MQTT.js (Node.js)

```javascript
const mqtt = require('mqtt');
const client = mqtt.connect('mqtt://localhost:1883');

client.on('connect', () => {
  client.subscribe('test/topic');
  client.publish('test/topic', 'Hello MQTT');
});

client.on('message', (topic, message) => {
  console.log(`收到消息: ${message.toString()}`);
});
```

## HTTP API 使用

### 获取状态

```bash
curl http://localhost:8080/api/status
```

### 发布消息

```bash
curl -X POST http://localhost:8080/api/publish \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "test/topic",
    "payload": "Hello from API",
    "qos": 1,
    "retain": false
  }'
```

### 获取客户端列表

```bash
curl http://localhost:8080/api/clients
```

## 集群配置

在 `config.yaml` 中启用集群：

```yaml
broker:
  cluster:
    enabled: true
    node_id: "node1"
    nodes:
      - "127.0.0.1:1884"
      - "127.0.0.1:1885"
    cluster_port: 1884
```

启动多个节点：

```bash
# 节点 1
go run main.go -config=config_node1.yaml

# 节点 2
go run main.go -config=config_node2.yaml
```

## 功能特性

### QoS 支持
- QoS 0: 最多一次
- QoS 1: 至少一次
- QoS 2: 恰好一次

### 离线消息
客户端断开后，QoS > 0 的消息会被保存，重连后自动发送。

### Retained 消息
发布时设置 `retain=true`，新订阅者会自动收到最后一条消息。

### Last Will
客户端连接时设置 Will 消息，异常断开时自动发布。

### 系统主题
订阅 `$SYS/#` 获取系统统计信息：
- `$SYS/broker/version`
- `$SYS/broker/uptime`
- `$SYS/broker/clients/connected`
- `$SYS/broker/messages/received`
- 等等...

## 故障排除

### 端口被占用
修改 `config.yaml` 中的端口配置。

### 连接失败
检查防火墙设置，确保端口已开放。

### 存储错误
检查 `data_path` 目录的写入权限。

## 性能调优

### 增加最大客户端数
```yaml
broker:
  max_clients: 50000
```

### 调整 Keep Alive
```yaml
broker:
  keep_alive: 600  # 10 分钟
```

### 优化批处理
修改 `internal/message/batch.go` 中的批处理参数。

