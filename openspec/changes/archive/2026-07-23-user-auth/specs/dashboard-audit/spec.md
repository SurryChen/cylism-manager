## 修改需求

### Requirement: 审计日志记录
系统应当自动将所有变更操作记录到审计日志，包含操作类型、资源类型、资源 ID、详情、时间戳和操作者 ID。

#### Scenario: 审计日志记录站点创建
- **WHEN** 已认证用户通过 API 创建站点
- **THEN** 系统写入审计日志条目，action="create"，resource_type="site"，resource_id=<新 ID>，detail 包含站点域名，user_id=<当前用户 ID>

#### Scenario: 审计日志包含操作者
- **WHEN** 已认证用户执行变更操作
- **THEN** 审计日志条目中 user_id 字段记录当前认证用户的 ID
