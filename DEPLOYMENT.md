# Scree-go WebRTC 应用部署指南

本文档提供了如何在本地环境和生产环境中部署 Scree-go WebRTC 屏幕共享应用的详细说明。

## 目录

- [环境要求](#环境要求)
- [本地开发部署](#本地开发部署)
- [Docker 部署](#docker-部署)
- [配置选项](#配置选项)
- [TURN 服务器配置](#turn-服务器配置)
- [安全注意事项](#安全注意事项)
- [故障排除](#故障排除)

## 环境要求

- Go 1.23+
- Docker (用于容器化部署)
- 公网 IP 地址或域名 (用于 TURN 服务器)

## 本地开发部署

### 1. 克隆仓库

```bash
git clone https://github.com/AsterZephyr/Scree-go-AZlearn.git
cd Scree-go-AZlearn
```

### 2. 安装依赖

```bash
go mod download
```

### 3. 创建配置文件

在项目根目录创建 `screego.config` 文件：

```env
# 基本配置
SCREEGO_EXTERNAL_IP=<您的公网IP或域名>
SCREEGO_LOG_LEVEL=info
SCREEGO_SERVER_ADDRESS=:5050

# TURN 服务器配置
SCREEGO_TURN_ADDRESS=:3478
SCREEGO_TURN_PORT_RANGE=50000:59999

# 认证配置
SCREEGO_AUTH_MODE=turn
SCREEGO_SECRET=<生成一个随机密钥>
```

### 4. 构建并运行

```bash
go build -o scree-go
./scree-go serve
```

应用将在 http://localhost:5050 上运行。

## Docker 部署

### 1. 构建 Docker 镜像

```bash
docker build -t scree-go:latest .
```

### 2. 运行 Docker 容器

```bash
docker run -d \
  --name scree-go \
  -p 5050:5050 \
  -p 3478:3478 \
  -p 3478:3478/udp \
  -p 50000-59999:50000-59999/udp \
  -e SCREEGO_EXTERNAL_IP=<您的公网IP或域名> \
  -e SCREEGO_SECRET=<生成一个随机密钥> \
  -v /path/to/config:/etc/screego \
  scree-go:latest
```

### 3. 使用 Docker Compose

创建 `docker-compose.yml` 文件：

```yaml
version: '3'

services:
  scree-go:
    build: .
    container_name: scree-go
    ports:
      - "5050:5050"
      - "3478:3478"
      - "3478:3478/udp"
      - "50000-59999:50000-59999/udp"
    environment:
      - SCREEGO_EXTERNAL_IP=<您的公网IP或域名>
      - SCREEGO_SECRET=<生成一个随机密钥>
      - SCREEGO_LOG_LEVEL=info
      - SCREEGO_AUTH_MODE=turn
    volumes:
      - ./config:/etc/screego
    restart: unless-stopped
```

然后运行：

```bash
docker-compose up -d
```

## 配置选项

以下是主要配置选项的说明：

| 环境变量 | 描述 | 默认值 |
|---------|------|-------|
| SCREEGO_EXTERNAL_IP | 服务器的公网 IP 地址或域名 | 必填 |
| SCREEGO_LOG_LEVEL | 日志级别 (debug, info, warn, error) | info |
| SCREEGO_SERVER_ADDRESS | HTTP 服务器监听地址 | :5050 |
| SCREEGO_SERVER_TLS | 是否启用 TLS | false |
| SCREEGO_TLS_CERT_FILE | TLS 证书文件路径 | |
| SCREEGO_TLS_KEY_FILE | TLS 密钥文件路径 | |
| SCREEGO_TURN_ADDRESS | TURN 服务器监听地址 | :3478 |
| SCREEGO_TURN_PORT_RANGE | TURN 服务器端口范围 | |
| SCREEGO_AUTH_MODE | 认证模式 (turn, all, none) | turn |
| SCREEGO_SECRET | 用于加密会话的密钥 | 随机生成 |
| SCREEGO_USERS_FILE | 用户文件路径 | |
| SCREEGO_TRUST_PROXY_HEADERS | 是否信任代理头 | false |
| SCREEGO_CORS_ALLOWED_ORIGINS | 允许的 CORS 源 | |
| SCREEGO_PROMETHEUS | 是否启用 Prometheus 指标 | false |

## TURN 服务器配置

TURN 服务器对于 NAT 穿透至关重要，特别是当用户位于严格的防火墙或对称型 NAT 后面时。

### 端口配置

确保以下端口在防火墙中开放：

- TCP 5050: Web 服务器
- TCP/UDP 3478: TURN 服务器
- UDP 50000-59999: TURN 媒体中继

### 外部 TURN 服务器

如果您想使用外部 TURN 服务器，可以设置：

```env
SCREEGO_TURN_EXTERNAL_IP=<外部TURN服务器IP>
SCREEGO_TURN_EXTERNAL_PORT=<外部TURN服务器端口>
SCREEGO_TURN_EXTERNAL_SECRET=<外部TURN服务器密钥>
```

## 安全注意事项

1. 始终在生产环境中启用 TLS
2. 使用强密码保护用户账户
3. 限制可以访问 TURN 服务器的 IP 地址
4. 定期更新 Docker 镜像和依赖

## 故障排除

### 连接问题

如果用户无法建立 WebRTC 连接：

1. 确保 TURN 服务器配置正确
2. 验证防火墙是否允许所需端口
3. 检查 `SCREEGO_EXTERNAL_IP` 是否设置为正确的公网 IP

### 日志分析

启用调试日志以获取更多信息：

```env
SCREEGO_LOG_LEVEL=debug
```

### 常见错误

- "cannot get external IP": 检查 `SCREEGO_EXTERNAL_IP` 配置
- "turn server failed to start": 检查 TURN 服务器端口是否被占用
- "session not found": WebRTC 会话创建失败，检查 TURN 配置
