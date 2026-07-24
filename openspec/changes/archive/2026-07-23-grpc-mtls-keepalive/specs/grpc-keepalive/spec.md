## ADDED Requirements

### Requirement: gRPC Keepalive 连接保持
系统 SHALL 为每个 gRPC 连接配置 keepalive 参数，每隔 10 秒发送探测 ping，3 秒超时。

#### Scenario: 连接空闲时自动探测
- **WHEN** gRPC 连接建立后无业务流量
- **THEN** 系统每 10 秒自动发送 TCP keepalive ping，保持连接活跃

### Requirement: gRPC 自动重连
系统 SHALL 在心跳 ping 失败时自动触发 gRPC 重连，而非直接关闭连接。

#### Scenario: 临时断线自动恢复
- **WHEN** Agent 短暂不可达导致心跳连续失败
- **THEN** 系统自动重新拨号，成功后将 failCount 重置，服务器状态保持 online

#### Scenario: 长时间断线标记 offline
- **WHEN** Agent 连续失败超过 10 次（约 5 分钟）
- **THEN** 系统将服务器状态标记为 offline，关闭连接
