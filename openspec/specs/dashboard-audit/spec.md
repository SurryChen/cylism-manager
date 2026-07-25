## Purpose
平台审计日志记录与仪表盘数据统计。
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

