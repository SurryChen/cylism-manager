## MODIFIED Requirements

### Requirement: 操作日志查询
系统 SHALL 同时提供按资源查询和全局查询操作历史，按时间倒序返回长流程步骤。按资源查询必须保持现有 `resource_type` 和 `resource_id` 参数契约；全局查询支持资源类型、状态、关键词和分页筛选。

#### Scenario: 查询服务器操作历史
- **WHEN** 用户请求 `GET /api/operations?resource_type=server&resource_id=1`
- **THEN** 系统返回该服务器所有操作步骤，按 created_at 倒序排列
- **AND** 每条记录包含 step、status、detail、created_at

#### Scenario: 查询全局操作历史
- **WHEN** 用户打开操作历史页面且未指定资源
- **THEN** 系统返回跨资源的长流程步骤，按 created_at 倒序分页
- **AND** 每条记录包含资源类型、资源 ID、步骤、状态、详情和时间

#### Scenario: 按状态筛选全局操作历史
- **WHEN** 用户筛选 status=`failed`
- **THEN** 系统仅返回状态为 failed 的操作步骤

#### Scenario: 查询时无关联日志
- **WHEN** 用户请求某资源的操作日志，但该资源尚无关联记录
- **THEN** 系统返回空数组和 200 状态码

## ADDED Requirements

### Requirement: 操作历史展示
系统 SHALL 在记录与系统中提供独立于审计日志的操作历史视图，展示长流程执行进度和结果。

#### Scenario: 操作历史默认展示流程步骤
- **WHEN** 用户打开操作历史视图
- **THEN** 页面展示时间、流程资源、步骤、状态和详情
- **AND** 页面不得将普通 HTTP 审计或 Agent 常规读取混入操作历史

#### Scenario: 从操作历史定位资源
- **WHEN** 一条操作历史关联可访问的资源
- **THEN** 用户可以通过该记录进入资源详情或其操作步骤视图
