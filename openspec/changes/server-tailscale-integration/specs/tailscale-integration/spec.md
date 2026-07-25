## ADDED Requirements

### Requirement: Tailscale 初始化
系统 SHALL 支持在 control-plane 节点上通过 Auth Key 一键初始化 Tailscale 网络。

#### Scenario: 首次初始化
- **WHEN** 用户提供有效的 Tailscale Auth Key 并触发初始化
- **THEN** 系统检测本机 Tailscale 是否已安装，若未安装则执行安装脚本；执行 `tailscale up --auth-key=$AUTH_KEY`；存储 Auth Key 加密后写入 system_configs 表；获取本机 Tailscale IP 并写入 DB

#### Scenario: 已初始化重复触发
- **WHEN** Tailscale 已安装且已注册到网络时触发初始化
- **THEN** 系统返回 "already initialized"，不重复执行安装

#### Scenario: Auth Key 无效
- **WHEN** 提供的 Auth Key 被 Tailscale 服务端拒绝
- **THEN** 系统返回错误信息，不存储无效 Key

### Requirement: Tailscale 状态查询
系统 SHALL 提供接口查询本机 Tailscale 运行状态。

#### Scenario: 查询状态
- **WHEN** 用户请求 Tailscale 状态
- **THEN** 系统返回 `tailscale ip -4` 输出、`tailscale status` 输出的节点列表、以及本机是否在线

### Requirement: Auth Key 管理
系统 SHALL 支持存储和更新 Tailscale Auth Key，Key 在持久化前加密。

#### Scenario: 更新 Auth Key
- **WHEN** 用户提交新的 Auth Key
- **THEN** 系统使用 AES-256 加密后写入 system_configs 表

#### Scenario: 读取 Auth Key 摘要
- **WHEN** 用户请求 Auth Key 信息
- **THEN** 系统返回脱敏后的 Key 摘要（如 `tskey-auth-kXxx...xxxx`）
