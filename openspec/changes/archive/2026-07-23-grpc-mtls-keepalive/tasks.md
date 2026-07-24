## 实现任务

### 1. CA 与证书生成模块
- [x] `internal/crypto/tls.go` 新增 `GenerateCA()`、`IssueCert()` 函数
- [x] 编写单元测试（CA 生成 + 签发的证书互相验证）

### 2. gRPC Pool keepalive + 重连
- [x] `internal/agent/pool.go` — Dial 增加 keepalive + connect params
- [x] 心跳重连逻辑：failCount 机制改为触发重连，10 次才 offline
- [x] 编写连接池测试

### 3. Agent gRPC Server mTLS
- [x] `cmd/agent/main.go` 新增 `--tls-cert`、`--tls-key`、`--tls-ca` flag
- [x] gRPC Server 启用 mTLS（`grpc.Creds`）
- [x] 编写 gRPC Server 测试

### 4. 平台启动时 CA 初始化
- [x] `cmd/platform/main.go` 启动时检查/生成 CA
- [x] `config/config.yaml` 新增 tls 配置段

### 5. 部署流程集成证书分发
- [x] `internal/api/router.go` — DeployAgent 时签发 + 上传证书
- [x] `internal/service/deployer/ssh_client.go` — 部署时写入证书 + agent mTLS 启动参数
- [x] 编写集成测试

### 6. 全量验证
- [x] `go test -v -count=1 ./...` 全部通过
- [x] `go build ./...` 编译通过
- [x] `cd web && npm run build` 前端构建通过
