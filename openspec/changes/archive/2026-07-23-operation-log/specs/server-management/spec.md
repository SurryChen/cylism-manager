## ADDED Requirements

### Requirement: 操作日志展示
系统 SHALL在服务器详情面板中展示操作日志，支持实时轮询更新。

#### Scenario: 服务器详情面板展示操作日志
- **WHEN** 用户打开服务器详情面板
- **THEN** 系统加载并展示该服务器的操作日志列表，按时间倒序排列，每条日志显示步骤名称、状态图标（加载中/成功/失败）、详情和时间

#### Scenario: 实时轮询操作日志
- **WHEN** 服务器详情面板中有 running 状态的操作日志
- **THEN** 系统每 2 秒轮询 GET /api/operations 接口，刷新日志列表

#### Scenario: 停止轮询
- **WHEN** 所有操作日志状态均为 success 或 failed，或用户关闭详情面板
- **THEN** 系统停止轮询
