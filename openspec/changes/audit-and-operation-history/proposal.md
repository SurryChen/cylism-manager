## Why

当前审计页混合了用户变更、Agent 普通读取和自动化过程记录。全局 HTTP 中间件又根据 URL 推断资源和动作，导致 `unknown / 创建` 这类没有运维语义的条目，真正需要追溯的变更、授权和失败事件被高频读取记录淹没。

平台需要把“谁做了什么安全或变更动作”与“一个长流程如何执行”分开呈现，并保留现有历史数据。

## What Changes

- 将审计日志定义为安全和配置变更的事实记录，提供明确的操作者、来源、目标、结果、摘要和关联标识。
- 以业务语义事件替代基于 HTTP URL 的资源/动作猜测；HTTP 中间件只保留认证、请求关联和委托上下文注入职责。
- 记录成功、失败和拒绝的关键动作，统一进行敏感字段白名单化和脱敏。
- 停止将 Agent 的普通读取、状态轮询和常规诊断默认写入审计；保留 Agent 的授权变更、操作申请、批准、拒绝、执行和失败等重要事件。
- 将现有 `operation_logs` 明确为操作历史，补充可在“记录与系统”中按时间查询的全局历史视图，同时保留资源详情的步骤查询。
- 重构前端为独立的“审计日志”和“操作历史”页面/标签，默认分别展示变更安全事件和长流程状态，不再直接展示原始 JSON 作为主列表内容。
- 为既有审计记录提供兼容回填和旧字段回退展示，不删除 `audit_logs`、`operation_logs` 或已有 API 路径。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `dashboard-audit`: 将审计从成功 HTTP 变更请求记录升级为具备明确业务语义、操作者、来源、目标和结果的安全与变更事件，并调整查询和展示要求。
- `operation-log`: 将操作日志明确为长流程操作历史，增加全局查询和前端历史视图，同时保留按资源查看操作步骤的能力。

## Impact

- 后端影响 `internal/model`、`internal/repository`、`internal/store`、审计中间件、关键 Handler/Service 的事件写入和 Bootstrap 组装。
- 需要对 SQLite 的 `audit_logs` 执行向后兼容的 schema migration/backfill；既有记录继续可读。
- API 影响审计列表 DTO/查询条件，以及新增或扩展全局操作历史查询接口；原有 `/api/audit-logs`、`/api/operations` 保持兼容。
- 前端影响 `web/src/views/AuditLogs.vue`、记录与系统导航、API 模块和测试。
- 不引入第三方依赖，不改变发布工作流本身或 Agent 权限模型。
