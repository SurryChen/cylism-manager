# operation-log Specification

## Purpose
TBD - created by archiving change operation-log. Update Purpose after archive.
## Requirements
### Requirement: 操作日志记录
系统 SHALL为长流程操作（部署 Agent、签发证书、续期证书、吊销证书、生成 NGINX 配置、重载 NGINX、导入 NGINX 配置、SSH 连通性测试）记录每一步的操作日志，包含操作步骤、状态（running/success/failed）、资源类型、资源 ID、详情和时间戳。

#### Scenario: 部署 Agent 操作日志
- **WHEN** 用户对服务器触发 Agent 部署
- **THEN** 系统按顺序写入操作日志：Step 1 "正在连接 SSH" (running)，Step 2 "正在检测操作系统" (running)，Step 3 "正在上传 Agent 二进制" (running)，Step 4 "正在启动 Agent" (running)，各步骤成功后分别更新为 success，若任一步骤失败则更新为 failed 并附加错误详情

#### Scenario: SSH 连通性测试操作日志
- **WHEN** 用户对服务器触发 SSH 连通性测试
- **THEN** 系统写入操作日志：Step 1 "正在测试 SSH 连通性" (running)，连接成功则更新为 success，连接失败则更新为 failed 并附加错误详情

#### Scenario: 操作日志关联资源
- **WHEN** 系统写入操作日志
- **THEN** 每条日志包含 resource_type（如 "server", "cert", "site"）和 resource_id，支持按任意资源类型查询

### Requirement: 操作日志查询
系统 SHALL提供通用 API 端点，支持按资源类型和资源 ID 查询操作日志，按时间倒序返回。

#### Scenario: 查询服务器操作日志
- **WHEN** 用户请求 GET /api/operations?resource_type=server&resource_id=1
- **THEN** 系统返回该服务器所有操作日志，按 created_at 倒序排列，每条包含 step、status、detail、created_at

#### Scenario: 查询时无关联日志
- **WHEN** 用户请求某资源的操作日志，但该资源尚无操作日志
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

