# MHMQTT - 高性能 MQTT Broker

一个功能完整的 MQTT broker 实现，使用 Go 语言开发。

## 功能特性

- ✅ 支持 MQTT v3.1, v3.1.1 及 v5.0 协议
- ✅ QoS 0/1/2 消息支持
- ✅ 离线消息支持
- ✅ Retained 消息支持
- ✅ Last Will 消息支持
- ✅ 分布式集群
- ✅ HTTP APIs
- ✅ $SYS 系统主题
- ✅ 存储会话信息
- ✅ 存储未过期消息
- ✅ 连接池优化
- ✅ 内存管理和批处理
- ✅ IO 优化（非阻塞 IO）

## 快速开始

### 安装依赖

```bash
go mod download
```

### 运行

```bash
go run main.go
```

### 配置

编辑 `config.yaml` 文件来配置 broker 参数。

## API 文档

HTTP API 默认监听在 `:8080` 端口。

### 获取 Broker 状态

```bash
GET /api/status
```

### 获取客户端列表

```bash
GET /api/clients
```

### 发布消息

```bash
POST /api/publish
Content-Type: application/json

{
  "topic": "test/topic",
  "payload": "Hello MQTT",
  "qos": 1,
  "retain": false
}
```

## 集群配置

在 `config.yaml` 中配置集群节点信息。

## 许可证

MIT

