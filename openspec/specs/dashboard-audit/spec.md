## Purpose
平台审计日志记录与仪表盘数据统计，覆盖用户操作追踪、资源变更留痕、后台清理策略以及面向运维人员的状态汇总展示能力。
## Requirements
### Requirement: 审计日志记录
系统 SHALL自动将所有变更操作记录到审计日志，包含操作类型、资源类型、资源 ID、详情、时间戳和操作者 ID。

#### Scenario: 审计日志记录站点创建
- **WHEN** 已认证用户通过 API 创建站点
- **THEN** 系统写入审计日志条目，action="create"，resource_type="site"，resource_id=<新 ID>，detail 包含站点域名，user_id=<当前用户 ID>

#### Scenario: 审计日志包含操作者
- **WHEN** 已认证用户执行变更操作
- **THEN** 审计日志条目中 user_id 字段记录当前认证用户的 ID

### Requirement: 操作日志清理任务
系统 SHALL在后台启动定时任务，按配置的保留天数自动清理过期操作日志。

#### Scenario: 启动时注册清理任务
- **WHEN** 系统启动
- **THEN** 系统启动一个后台 goroutine，每小时执行一次操作日志清理

#### Scenario: 清理过期日志
- **WHEN** 清理任务运行且配置 retention_days > 0
- **THEN** 系统删除所有 created_at 早于（当前时间 - retention_days 天）的操作日志记录

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: 概览页引导与集群总览
系统 SHALL 在概览页展示 tailnet 连接状态、control-plane 状态、集群资源总览、待激活服务器数和最近操作日志，并在关键前置条件缺失时展示引导卡片。

#### Scenario: 缺少 tailnet 或 join token 引导
- **WHEN** 系统检测到无法读取本机 tailnet 状态，或缺少 `k3s_join_token`
- **THEN** 概览页显示引导卡片，提示用户前往“服务器”或“系统设置”完成配置

#### Scenario: 展示集群摘要
- **WHEN** 用户打开概览页
- **THEN** 页面展示节点数、命名空间数、工作负载数、服务数和异常节点数

### Requirement: 高风险操作审计覆盖
系统 SHALL 将导入服务器、更新 SSH 凭据、激活服务器、加入 worker、移除节点、查看 Secret 明文和启用扩展纳入审计日志。

#### Scenario: 激活服务器写审计
- **WHEN** 已认证用户触发服务器激活检测
- **THEN** 系统写入审计日志，包含 action、resource_type、resource_id、result 和 user_id

#### Scenario: 查看 Secret 明文写审计
- **WHEN** 已认证用户在配置页面确认查看某 Secret 的明文值
- **THEN** 系统写入一条查看敏感数据的审计日志
