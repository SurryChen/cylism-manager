## ADDED Requirements

### Requirement: WebSocket 实时进度推送
系统 SHALL 通过 WebSocket 连接实时推送长流程操作的进度日志。

#### Scenario: 加入集群进度推送
- **WHEN** 用户通过 WebSocket 连接 `/api/nodes/:id/join-progress`
- **THEN** 系统按序推送 12 条进度消息，每条包含 step/label/status/detail/index/total/ts 字段，status 为 running/success/failed

#### Scenario: 步骤失败中断
- **WHEN** 某步骤执行失败（如 SSH 断开）
- **THEN** 系统推送该步骤的 status=failed 消息，随后关闭 WebSocket 连接

#### Scenario: 客户端提前断开
- **WHEN** 用户在流程未完成时关闭弹窗
- **THEN** 服务端检测到连接关闭后，终止后续操作（best-effort）

### Requirement: 进度消息格式
所有 WebSocket 进度消息 SHALL 遵循统一格式。

#### Scenario: 消息格式验证
- **WHEN** 任意步骤推送消息
- **THEN** 消息 JSON 包含必填字段：`step`(string), `label`(string), `status`(enum), `detail`(string), `index`(int), `total`(int), `ts`(RFC3339 string)
