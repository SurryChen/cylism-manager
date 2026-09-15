## Context

当前 `audit_logs` 只有 `action`、资源类型/ID、用户 ID 和 JSON detail。`AuditMiddleware` 针对所有成功的 `POST`、`PUT`、`PATCH`、`DELETE` 请求写入一条记录，并由 URL 推断动作和资源。推断只覆盖少数路径，所以节点镜像源验证等记录会显示为 `unknown / 创建`。Agent Handler 还会对普通读取能力显式写入 `agent.pod_get`、`agent.event_list` 等事件，造成审计页噪声。

`operation_logs` 已用于应用发布等长流程的运行步骤，但仅支持按 `resource_type` 与 `resource_id` 查询，缺少面向运维人员的全局历史入口。两张表均已存在生产数据，迁移必须无损。

## Goals / Non-Goals

**Goals:**

- 用可读、可筛选、可关联的业务事件记录安全和变更事实。
- 将长流程步骤与安全/变更审计分别查询和展示。
- 覆盖用户、委托会话、Agent 和系统自动化四种事件来源，明确成功、失败和拒绝结果。
- 不再让普通 Agent 读取污染默认审计列表。
- 保留旧数据、旧 API 路径和资源详情中的操作步骤查询。

**Non-Goals:**

- 不记录所有 HTTP GET、Dashboard 轮询或完整 HTTP 响应体。
- 不做 SIEM、外部日志投递、不可篡改存证、跨集群审计聚合或实时告警。
- 不改变 Agent 已授予能力、发布执行步骤或资源生命周期。
- 不在本 Change 中清理历史审计数据，也不新增统一的全局日志保留策略。

## Decisions

### 1. 保留两类持久化记录，明确边界

`audit_logs` 保留为审计事件：授权、安全、人工或自动化变更，以及这些动作的失败/拒绝。`operation_logs` 保留为长流程每个阶段的状态历史，例如发布预检、镜像拉取、资源应用、等待就绪和回滚。

审计回答“谁对什么做了什么，结果如何”；操作历史回答“该流程走到哪里、每一步结果如何”。同一发布可以同时有一条 `application.release.requested` 审计事件和多条 Release 操作历史。

备选方案：合并为一张事件表。这样全局查询简单，但会将一次发布的几十个步骤与一条授权/变更事实混在一起，默认视图仍然会失焦，因此不采用。

### 2. 审计采用类型化业务事件，HTTP 中间件只提供上下文

引入由 service/handler 使用的 `AuditService.Record(ctx, AuditEventInput)`。输入必须包含 action、target、outcome、summary，并从 Context 补充操作者、来源、请求 ID 和委托 ID。事件使用稳定的领域动作名，如 `registry.mirror.verify`、`application.release.create`、`agent.operation.approve`。

全局 HTTP 中间件不再根据路径自动写审计记录；它负责创建/透传 request ID，恢复请求体可读性，并把认证用户和 delegation 关联信息提供给业务写入点。为防止漏记，将关键受保护变更路由列为覆盖清单并通过路由/服务测试验证。

备选方案：扩展 `inferResourceType` 和 `inferAction` 的 URL 规则。它能快速修复 `unknown`，但仍无法可靠区分“验证”“申请”“批准”“失败”，也无法记录异步执行结果，因而不采用。

### 3. 在既有 AuditLog 上增量扩展并回填兼容值

`AuditLog` 增加：`actor_type`、`actor_name`、`source`、`outcome`、`summary`、`target_name`、`request_id`、`operation_id`。保留现有 `Action`、`ResourceType`、`ResourceID`、`UserID` 和 `Detail`。

SQLite AutoMigrate 负责增列；启动 backfill 为旧记录写入 `source=legacy`、`outcome=succeeded`、可读的 fallback summary。列表 DTO 对未回填或旧数据仍根据老字段生成兼容显示。新增列初期允许空值，回填完成后新事件必须完整写入要求字段。

备选方案：新建 `audit_events` 表并复制旧数据。它能获得更干净的模型，但需要双表读取、切换和长期兼容，且没有必要承担数据丢失风险，因此不采用。

### 4. 明确记录范围与结果语义

必须记录：认证与委托、权限授予/撤销、敏感数据访问、资源创建/更新/删除、部署/回滚/扩缩容、镜像源验证、Terminal 会话、Agent 操作申请/批准/拒绝/执行/失败。

不默认记录：Agent 普通 read/list/status、监控查询、状态轮询和一般健康检查。读操作只有在被拒绝、访问敏感数据或由人工发起的高风险诊断时才记录。结果统一为 `succeeded`、`failed`、`denied`；异步操作申请为 `accepted`，最终结果由执行完成事件关联同一 operation ID。

元数据使用每类事件的白名单 DTO；不再保存整个 HTTP request body。Secret、password、token、private key、ConfigMap content 继续脱敏，审计测试必须证明值不泄露。

### 5. 记录与系统提供两个独立视图

“审计日志”默认筛选变更与安全事件，主列表为：时间、结果、动作、目标、操作者、摘要。详情页才展示来源、请求 ID、操作 ID 与结构化元数据。

“操作历史”展示全局长流程步骤，支持按资源类型、资源、状态和关键词筛选，主列表为：时间、流程/资源、步骤、状态、详情，并可进入关联资源。现有资源详情使用的 `/api/operations?resource_type=&resource_id=` 继续返回相同步骤结构；新增全局查询不要求 resource 参数。

备选方案：继续在一个审计页增加复杂筛选。它会让用户先理解数据来源和日志级别才能检索，无法形成清晰的默认工作流，因此不采用。

## Risks / Trade-offs

- [迁移遗漏关键变更写入点] → 建立关键路由/服务覆盖清单，使用 table-driven tests 验证每类事件的 action、target、actor、outcome。
- [异步操作出现“申请成功但最终失败”] → 以同一 `operation_id` 写入 accepted 和 failed/succeeded 两个审计事件，前端显示独立结果且可关联查看。
- [旧数据字段不完整] → 所有新 DTO 提供 legacy fallback；回填失败不阻止启动且保留原 detail。
- [审计写入失败影响主业务] → 安全和权限变更写入失败必须使请求失败；普通资源变更先以同步写入为目标，失败记录结构化服务日志并在实施时明确每类操作的失败策略。
- [事件量继续增长] → 普通 Agent 读取不写入审计；列表分页和索引覆盖常用筛选字段，保留策略另行设计。
- [敏感内容泄漏] → 使用事件元数据白名单，而非原始 body；保留并扩展递归脱敏测试。

## Migration Plan

1. 为 AuditLog 新增兼容字段、索引和启动回填，先补 migration/backfill 测试。
2. 引入 AuditService、Context 关联信息和 DTO/query 测试，不改变页面默认入口。
3. 逐域迁移认证、项目/应用、基础设施、交付、Agent 操作和 Terminal 的关键事件；移除 Agent 普通读取写入。
4. 扩展操作历史全局查询，保持既有按资源查询契约。
5. 上线前运行全量 Go/Vue 测试与构建；通过旧数据库 fixture 验证历史审计可查。
6. 发布后观察 `legacy`、`unknown` 和 Agent read 动作数量；异常时可先恢复旧中间件写入作为临时回退，不回滚数据结构。

## Open Questions

- 审计保留天数是否需要独立于现有操作历史保留天数配置；本 Change 不改变默认保留行为。
- 是否在后续 Change 中增加用户显示名快照与客户端 IP/User-Agent；当前先使用用户 ID、已知 runtime 名称和来源。
