## Why

当前 gRPC 通信存在两个关键缺陷：
1. 无 keepalive 和重连机制：心跳失败 3 次后连接被永久关闭，Agent 断开后无法自动恢复
2. 公网明文传输（insecure credentials）：gRPC 流量可被中间人窃听和篡改

## What Changes

- gRPC dial 增加 keepalive 参数（ClientParameters），启用 TCP keepalive 探测
- 心跳失败触发自动重连（grpc.Connect + WaitForStateChange），替代当前"3 次失败永久关闭"
- 平台侧新增 CA 签发自签名证书能力，部署 Agent 时自动签发客户端证书
- gRPC Client/Server 启用 mTLS 双向认证
- Agent 启动参数新增 `--tls-cert`、`--tls-key`、`--tls-ca`，部署时写入证书文件

## Capabilities

### 新增能力

- `grpc-security`: gRPC mTLS 双向认证，平台自签名 CA 签发证书
- `grpc-keepalive`: gRPC 连接 keepalive 与自动重连

### 修改的能力

- `server-management`: 部署流程增加证书签发与分发
- `agent`: Agent 启动参数扩展，支持 mTLS

## 影响范围

- `internal/agent/pool.go` — 增加 keepalive 参数 + 重连逻辑
- `internal/crypto/` — 新增 CA/证书生成函数
- `cmd/platform/main.go` — 启动时生成/加载 CA
- `cmd/agent/main.go` — 新增 --tls-* 参数 + mTLS gRPC Server
- `internal/api/router.go` — 部署时签发证书并分发到远端
- `internal/service/deployer/ssh_client.go` — 部署时上传证书 + agent 启动参数
- `config/config.yaml` — 新增 tls 配置段
