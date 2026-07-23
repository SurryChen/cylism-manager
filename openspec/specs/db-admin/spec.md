# db-admin Specification

## Purpose
TBD - created by archiving change db-admin. Update Purpose after archive.
## Requirements
### Requirement: 表列表查询
系统 SHALL 提供 API 返回数据库中所有可管理的数据表名称。

#### Scenario: 获取表列表
- **WHEN** 已认证管理员请求 GET /api/admin/tables
- **THEN** 系统返回 `["servers","sites","certs","audit_logs","operation_logs","users"]`

### Requirement: 表数据分页查询
系统 SHALL 支持按表名分页查询数据，支持指定排序列和排序方向。

#### Scenario: 分页查询第一页
- **WHEN** 管理员请求 GET /api/admin/tables/servers?page=1&size=20
- **THEN** 系统返回该表前 20 条记录及总条数

#### Scenario: 按列排序
- **WHEN** 管理员请求 GET /api/admin/tables/servers?page=1&size=20&sort=name&order=asc
- **THEN** 系统按 name 升序返回数据

#### Scenario: 敏感字段不可见
- **WHEN** 管理员查询 users 表
- **THEN** 返回的数据中不包含 password_hash 字段

### Requirement: 新增记录
系统 SHALL 支持向指定表插入新记录，自动排除 id、created_at、updated_at、deleted_at 字段。

#### Scenario: 新增服务器记录
- **WHEN** 管理员提交 POST /api/admin/tables/servers，body 包含 name 和 host
- **THEN** 系统创建新记录并返回完整的行数据

#### Scenario: 新增记录缺少必填字段
- **WHEN** 管理员提交新增请求但缺少 NOT NULL 字段
- **THEN** 系统返回 400 错误，提示具体缺失字段

### Requirement: 更新记录
系统 SHALL 支持按表名和主键 ID 更新记录。

#### Scenario: 更新服务器名称
- **WHEN** 管理员提交 PUT /api/admin/tables/servers/1，body 包含 name="new-name"
- **THEN** 系统更新该记录并返回更新后的数据

### Requirement: 删除记录
系统 SHALL 支持按表名和主键 ID 删除记录。

#### Scenario: 删除记录
- **WHEN** 管理员提交 DELETE /api/admin/tables/servers/1
- **THEN** 系统删除该记录并返回 ok

