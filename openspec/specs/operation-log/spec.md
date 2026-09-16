# operation-log Specification

## Purpose
TBD - created by archiving change operation-log. Update Purpose after archive.
## Requirements
### Requirement: 操作日志记录
系统 SHALL 为长流程操作（部署 Agent、签发证书、续期证书、吊销证书、生成 NGINX 配置、重载 NGINX、导入 NGINX 配置、SSH 连通性测试、应用发布、发布重试和发布回滚）记录每一步的操作日志，包含操作步骤、状态（running/success/failed）、资源类型、资源 ID、详情和时间戳。

#### Scenario: 部署 Agent 操作日志
- **WHEN** 用户对服务器触发 Agent 部署
- **THEN** 系统按顺序写入操作日志：Step 1 "正在连接 SSH" (running)，Step 2 "正在检测操作系统" (running)，Step 3 "正在上传 Agent 二进制" (running)，Step 4 "正在启动 Agent" (running)，各步骤成功后分别更新为 success，若任一步骤失败则更新为 failed 并附加错误详情

#### Scenario: SSH 连通性测试操作日志
- **WHEN** 用户对服务器触发 SSH 连通性测试
- **THEN** 系统写入操作日志：Step 1 "正在测试 SSH 连通性" (running)，连接成功则更新为 success，连接失败则更新为 failed 并附加错误详情

#### Scenario: 应用发布操作日志
- **WHEN** 用户创建、重试或回滚一个 Application Release
- **THEN** 系统以 `resource_type=release` 和 Release ID 写入预检、配置、工作负载、服务、证书、路由和验证步骤的操作日志
- **AND** 日志详情不得包含 Secret 明文

#### Scenario: 操作日志关联资源
- **WHEN** 系统写入操作日志
- **THEN** 每条日志包含 resource_type（如 "server", "cert", "site", "release"）和 resource_id，支持按任意资源类型查询

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

### Requirement: 操作日志自动清理
系统 SHALL根据可配置的保留天数自动清理过期操作日志。

#### Scenario: 按保留天数清理日志
- **WHEN** 配置 retention_days=30，且存在超过 30 天的操作日志
- **THEN** 系统每小时检查一次，删除 created_at 早于 30 天前的所有操作日志

#### Scenario: 永不过期配置
- **WHEN** 配置 retention_days=0
- **THEN** 系统不自动清理任何操作日志

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: 操作历史展示
系统 SHALL 在记录与系统中提供独立于审计日志的操作历史视图，展示长流程执行进度和结果。

#### Scenario: 操作历史默认展示流程步骤
- **WHEN** 用户打开操作历史视图
- **THEN** 页面展示时间、流程资源、步骤、状态和详情
- **AND** 页面不得将普通 HTTP 审计或 Agent 常规读取混入操作历史

#### Scenario: 从操作历史定位资源
- **WHEN** 一条操作历史关联可访问的资源
- **THEN** 用户可以通过该记录进入资源详情或其操作步骤视图

