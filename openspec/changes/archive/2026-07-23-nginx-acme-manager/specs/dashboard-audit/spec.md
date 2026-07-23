## 新增需求

### Requirement: 仪表盘概览
系统应当提供仪表盘 API 端点，返回摘要统计，包括服务器总数、站点总数、即将在 30 天内到期的证书以及最近活动。

#### Scenario: 混合状态仪表盘
- **WHEN** 用户请求仪表盘
- **THEN** 系统返回 JSON，包含在线/离线服务器数量、站点总数、即将到期证书数以及最近 10 条审计日志

### Requirement: 审计日志记录
系统应当自动将所有变更操作（创建、更新、删除、签发、续期、吊销、部署、重载）记录到审计日志，包含操作类型、资源类型、资源 ID、详情和时间戳。

#### Scenario: 审计日志记录站点创建
- **WHEN** 通过 API 创建站点
- **THEN** 系统写入审计日志条目，action="create"，resource_type="site"，resource_id=<新 ID>，detail 包含站点域名

#### Scenario: 审计日志记录证书签发
- **WHEN** 证书签发
- **THEN** 系统写入审计日志条目，action="issue"，resource_type="cert"，resource_id=<证书 ID>，detail 包含域名

### Requirement: 审计日志查询
系统应当提供审计日志查询 API 端点，支持分页以及按资源类型和操作类型筛选。

#### Scenario: 查询最近审计日志
- **WHEN** 用户以默认分页请求审计日志
- **THEN** 系统返回最近 20 条日志条目，按时间倒序排列

#### Scenario: 按资源类型筛选审计日志
- **WHEN** 用户请求按 resource_type="cert" 筛选的审计日志
- **THEN** 系统仅返回与证书相关的日志条目
