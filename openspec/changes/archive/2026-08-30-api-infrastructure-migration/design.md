## Context

基础设施代码目前分散在十余个 API 文件中：服务器和节点涉及 SSH、Agent 与 K8s；PVC 包含创建、用量、主机目录导入和跨节点迁移；网络包含受管域名、证书、Ingress 与 CoreDNS 配置。它们需要复用相同的 Store 与 Kubernetes 访问能力，但不应通过 Handler 相互调用。

## Goals / Non-Goals

**Goals:**

- 让 HTTP Handler 只处理参数、鉴权上下文和统一响应映射。
- 将每个基础设施用例收敛到可脱离 Gin 测试的 Service。
- 将多领域复用的 Kubernetes 资源读写放入 `internal/k8s`。
- 每完成一个领域即保持路由、错误码、响应字段与异步任务行为兼容。

**Non-Goals:**

- 不重写既有 UI、不改变 REST 路径或数据库结构。
- 不在单次迁移中合并服务器、节点、存储与网络的业务规则。
- 不将 Agent、审计和监控横向能力提前纳入本阶段。

## Decisions

### 按风险分批迁移，而非一次移动全部文件

顺序为：服务器与节点、存储、网络。每个模块先定义 Service 测试，再移动 Handler 与测试，完成全量验证后才开始下一个模块。

备选方案是移动所有文件后统一修复编译问题；该方案无法清楚定位契约回归，也会扩大发布风险。

### 以领域 Service 编排，Kubernetes Adapter 只操作资源

`cluster` Service 负责服务器与节点用例，`storage` Service 负责 PVC 生命周期和迁移，`network` Service 负责域名/证书/Ingress/DNS 编排。`internal/k8s` 保持资源查询、apply、等待和状态读取，不接收 Gin 类型，也不掌握 HTTP 错误码。

备选方案是让每个 Handler 直接使用 K8s Client；这会继续复制校验和异步任务管理，无法被 CLI/Agent 复用。

### 通过构造函数注入副作用

Service 接收 Store、K8s Adapter 和最小化的 SSH/Agent 回调接口。测试用 fake 替代副作用；路由层负责提供生产适配器。全局 `api.K8s` 只作为迁移期构造依赖来源，不再被已迁移 Handler 直接编排使用。

### 保持渐进式目录迁移

领域完成时将 Handler 迁移到 `internal/api/infrastructure/`；未迁移 API 继续保留在根包。禁止 API 子包互调业务逻辑。

## Risks / Trade-offs

- [长耗时 PVC 导入/迁移任务因重构改变恢复语义] → 先用现有集成测试锁定任务状态、回滚和清理路径，再抽取后台任务。
- [节点排空或删除错误码变化] → 保留现有 Handler 错误映射，Service 使用可分类错误并补充路由级回归测试。
- [循环依赖] → Service 仅依赖 Store、Model、K8s 与显式接口，禁止依赖 `internal/api`。
- [移动范围过大] → 每个领域独立提交；任一领域验证失败时可独立回退。

## Migration Plan

1. 为服务器/节点的 Service 定义测试和构造依赖，迁移 HTTP Adapter 与路由。
2. 为 PVC 清单、创建、导入、迁移与保护逻辑建立 Storage Service，迁移后验证后台任务恢复语义。
3. 迁移域名、证书、Ingress 与 DNS 的 Network Service，保持加密凭据和证书生命周期安全边界。
4. 执行 `go test ./...`、`go build ./...`、Node 24 前端构建、OpenSpec 严格校验和安全扫描；得到确认后归档。

## Open Questions

- 服务器的 SSH 探测与 Agent 诊断是否在阶段五保持一个 `cluster` Service，或在阶段六随 Agent 代码进一步拆分；首期采用一个 Service 和窄接口，避免复制连接前置检查。
