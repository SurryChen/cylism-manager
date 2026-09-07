## MODIFIED Requirements

### Requirement: 服务器列表
系统 SHALL 返回所有已注册服务器的列表，包含 SSH 凭据信息和 Tailscale 状态。

#### Scenario: 列出所有服务器
- **WHEN** 用户请求服务器列表
- **THEN** 系统返回所有服务器，JSON 包含 `ssh_user`、`ssh_auth_type`、`tailscale_ip`、`tailscale_online` 字段，不包含 `status`、`last_seen`、`sites` 字段

### Requirement: 服务器注册
系统 SHALL 允许用户注册远程服务器，提供名称、主机地址、SSH 连接参数。

#### Scenario: 使用密码认证注册服务器
- **WHEN** 用户提交服务器注册，名称为 "web-01"，主机为 "10.0.0.1"，SSH 用户为 "root"，SSH 密码为指定值
- **THEN** 系统存储服务器记录，并在持久化前使用 AES-256 加密密码；新记录的 `tailscale_ip` 为空，`tailscale_online` 为 false

## REMOVED Requirements

### Requirement: 服务器状态监控
**Reason**: `Status` 和 `LastSeen` 字段依赖 gRPC Agent 心跳，该项目已不再维护 gRPC Agent 机制，且字段从未被可靠更新。

### Requirement: Agent 部署
**Reason**: 该项目已从 gRPC Agent 架构迁移至纯 SSH + k3s 方案，Agent 部署流程不再适用。
