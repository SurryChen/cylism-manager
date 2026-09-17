## 1. 审计模型与兼容迁移

- [x] 1.1 为 AuditLog 扩展操作者、来源、结果、摘要、目标名称和关联标识字段，并补充 SQLite migration/backfill 测试以证明旧记录可读。
- [x] 1.2 扩展审计 repository 查询条件与 DTO，覆盖结果、动作、目标、操作者、来源、关键词、分页和 legacy fallback 的单元测试。
- [x] 1.3 实现类型化 AuditService 与请求上下文关联，覆盖脱敏与事件完整性。

## 2. 语义化审计写入

- [x] 2.1 将全局 AuditMiddleware 改为请求关联上下文注入，移除成功变更请求的 URL 推断写入，并测试不再产生 unknown/create 推断事件。
- [x] 2.2 为认证、委托、权限授予/撤销和敏感数据访问补充语义化审计事件及测试。
- [x] 2.3 为项目、应用、发布/回滚、基础设施、交付和镜像源验证等关键变更补充事件写入及路由/Handler 测试。
- [x] 2.4 为 Terminal 会话和 Agent 操作申请、批准、拒绝、执行、失败补充关联 operation_id 的事件及测试。
- [x] 2.5 删除 Agent 普通 read/list/status 的审计写入，保留拒绝、高风险诊断和执行型事件，并编写回归测试。

## 3. 操作历史查询

- [x] 3.1 扩展 OperationLog repository 支持全局分页、资源类型、状态和关键词查询，并保持按资源查询 API 契约不变。
- [x] 3.2 扩展操作历史 Handler/DTO/API，编写全局与按资源查询、空结果和失败筛选测试。

## 4. 记录与系统前端

- [x] 4.1 重构审计日志 API 模块和视图，展示结果、动作、目标、操作者和摘要，支持结构化详情与新增筛选条件。
- [x] 4.2 新增独立操作历史视图/API 模块，将记录与系统导航拆分为审计日志和操作历史入口。
- [x] 4.3 补充 Vue 测试，覆盖默认列表、筛选、legacy 显示和两个视图的独立数据来源。

## 5. 验证与迁移检查

- [x] 5.1 运行相关 Go package 测试，验证旧 SQLite 数据库迁移、审计脱敏和关键路由事件覆盖。
- [x] 5.2 运行 `go test ./...`、`go build ./...`、前端全量测试和 `npm run build`。
- [x] 5.3 运行 `git diff --check` 和 `openspec validate audit-and-operation-history`，在验证通过后提交审查结果。
