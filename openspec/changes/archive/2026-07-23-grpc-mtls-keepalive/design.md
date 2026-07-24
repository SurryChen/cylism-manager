## 背景

当前 gRPC 使用 `insecure.NewCredentials()` 明文传输，无 keepalive，心跳失败 3 次后连接被永久关闭。公网场景下，流量可被窃听，Agent 断开后需手动"状态同步"才能恢复。

## 目标 / 非目标

**目标：**
- gRPC 连接增加 keepalive 探测，断线自动重连
- 平台签发自签名 CA，部署时自动为 Agent 签发证书
- gRPC Client/Server 启用 mTLS 双向认证
- 部署流程自动分发证书到远端

**非目标：**
- 证书自动续期（一期证书有效期设为 10 年，过期后重新部署即可）
- 外部 CA 集成（Let's Encrypt 等）
- CRL/OCSP 吊销机制

## 技术决策

### 1. gRPC Keepalive 参数

```go
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                10 * time.Second,  // 10s 发一次 ping
    Timeout:             3 * time.Second,   // ping 超时
    PermitWithoutStream: true,              // 无活跃流也发 ping
})
```

配合 `grpc.WithConnectParams` 设置重连退避。

### 2. 自动重连策略

当前的 `failCount >= 3 → Close + delete` 改为：

```
心跳 ping 失败
  → 检查 conn 状态 (grpc.ConnectivityState)
  → 若 TransientFailure 或 Shutdown → 触发 grpc.Dial 重新连接
  → 新连接成功后 reset failCount
  → 只有连续失败超过 10 次（5 分钟）才标记 offline + 关闭
```

### 3. CA 与证书管理

平台启动时检查 `data/tls/`：

```
data/tls/
  ca-cert.pem    # CA 证书（不存在则自动生成）
  ca-key.pem     # CA 私钥
  servers/
    1-cert.pem   # 服务器 1 的 Agent 证书
    1-key.pem    # 服务器 1 的 Agent 私钥
```

生成逻辑：`crypto/x509` 标准库，ECDSA P256 密钥对。

### 4. 部署流程中的证书分发

```
DeployAgent:
  1. 为 Agent 签发证书（data/tls/servers/{id}-cert.pem, {id}-key.pem）
  2. SSH 上传 ca-cert.pem + agent-cert.pem + agent-key.pem 到远端
  3. 启动 Agent 时增加参数：
       --tls-cert=/opt/cylism-manager/cert.pem
       --tls-key=/opt/cylism-manager/key.pem
       --tls-ca=/opt/cylism-manager/ca.pem
```

### 5. 配置项

```yaml
tls:
  ca_cert: "data/tls/ca-cert.pem"
  ca_key: "data/tls/ca-key.pem"
  server_cert_dir: "data/tls/servers"
  validity_days: 3650  # 10 年
```

### 6. gRPC Server (Agent) mTLS

```go
tlsConfig := &tls.Config{
    ClientAuth: tls.RequireAndVerifyClientCert,
    ClientCAs:  caCertPool,
    Certificates: []tls.Certificate{serverCert},
}
grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
```

## 风险与权衡

- **[证书管理]** 每台服务器一对证书，文件数量随服务器增长 → 一期在 20 台以内可接受
- **[Agent 启动失败]** 证书文件缺失 → Agent 报错退出，需重新部署
- **[版本兼容]** 旧 Agent 不支持 mTLS → 部署新 Agent 时自然升级
