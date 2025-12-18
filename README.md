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

## Docker 部署

### 构建镜像

```bash
docker build -t mhmqtt .
```

### 运行容器

```bash
docker run -d \
  -p 1883:1883 \
  -p 8083:8083 \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  --name mhmqtt \
  mhmqtt
```

## 自动化分发 (CI/CD)

本项目已配置 GitHub Actions 自动构建和发布 Docker 镜像。

### 配置步骤

1. 在 GitHub 仓库设置中，进入 `Settings` -> `Secrets and variables` -> `Actions`。
2. 添加以下 Repository Secrets：
   - `DOCKER_USERNAME`: 你的 Docker Hub 用户名
   - `DOCKER_PASSWORD`: 你的 Docker Hub 访问令牌 (Access Token) 或密码

配置完成后，每次推送到 `main` 分支或打上 `v*.*.*` 标签时，会自动构建并推送到 Docker Hub。

## 订阅模式

- 普通订阅：`sensor/+/temp`、`devices/#` 等
- 共享订阅：`$share/<group>/<filter>`，同一 group 内的多个客户端将以轮询方式分担消息
- 队列订阅（别名）：`$queue/<filter>` 等价于 `$share/queue/<filter>`

### 共享订阅示例

```text
$share/workers/sensor/+/temp
```

- 客户端 A、B 都订阅上述主题时，发布到 `sensor/x/temp` 的消息会在 A、B 之间轮询分发
- 支持通配符匹配
- Retained 消息在订阅建立时按过滤器匹配并下发到当前订阅者

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
