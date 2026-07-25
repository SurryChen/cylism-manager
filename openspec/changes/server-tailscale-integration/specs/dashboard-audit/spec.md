## MODIFIED Requirements

### Requirement: Dashboard 统计
系统 SHALL 提供仪表盘概览统计数据，包含服务器数、站点数、即将到期证书数。

#### Scenario: 获取统计数据
- **WHEN** 用户请求 Dashboard 概览
- **THEN** 系统返回 `total_servers`、`total_sites`、`expiring_certs`，不返回 `online_servers`

#### Scenario: Tailscale 未初始化提示
- **WHEN** Dashboard 加载且 Tailscale 未初始化
- **THEN** 前端展示 Tailscale 初始化 Banner，引导用户配置 Auth Key
