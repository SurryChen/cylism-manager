## MODIFIED Requirements

### Requirement: 服务器列表与详情
系统 SHALL 提供服务器列表与详情能力，返回服务器台账、用户提供的管理地址、SSH 配置、激活状态和主机事实缓存摘要；系统 SHALL 不再把 Tailscale 纳管信息或 Agent 心跳状态作为列表主状态。

#### Scenario: 列出所有服务器
- **WHEN** 用户请求服务器列表
- **THEN** 系统返回名称、管理地址、SSH 用户、权限模式、激活状态和最近采集时间
- **AND THEN** 响应中不包含明文凭据、Tailscale Auth Key 或 Tailscale 设备元数据

#### Scenario: 查看服务器详情
- **WHEN** 用户请求特定服务器详情
- **THEN** 系统返回完整台账信息、脱敏后的 SSH 配置、最近一次激活结果和主机事实缓存
- **AND THEN** 系统不将管理地址解释为特定网络提供商地址

## REMOVED Requirements

### Requirement: Tailnet 服务器导入
**Reason**: 平台不再访问宿主机 Tailscale socket 或管理 Tailnet peer；主机注册使用操作员提供的可达管理地址。

**Migration**: Register hosts manually with their reachable address and SSH credentials. Existing server records remain usable through their stored management address.
