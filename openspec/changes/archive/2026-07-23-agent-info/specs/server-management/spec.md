## ADDED Requirements

### Requirement: Agent 信息卡片展示
系统 SHALL 在服务器详情面板中展示 Agent 信息卡片，包含状态、版本、部署路径、最后部署时间和最后在线时间。

#### Scenario: 已部署 Agent 的服务器详情
- **WHEN** 用户点击已部署 Agent 的服务器行
- **THEN** 详情面板展示 Agent 信息卡片：在线/离线状态、Agent 版本、部署路径、最后部署时间和最后在线时间

#### Scenario: 未部署 Agent 的服务器详情
- **WHEN** 用户点击未部署 Agent 的服务器行
- **THEN** 详情面板展示 Agent 信息卡片，版本和部署路径显示为"未部署"

### Requirement: 状态同步操作入口
系统 SHALL 在服务器列表每行提供"状态同步"按钮。

#### Scenario: 点击状态同步
- **WHEN** 用户点击某服务器的"状态同步"按钮
- **THEN** 系统调用 ProbeAgent 探测远端 Agent，若运行中则更新状态为 online 并刷新列表
