# Design: Handler Composition Migration

## Composition Boundary

`cmd/platform` 只读取配置、创建 Container、注册路由和处理进程退出。`bootstrap.Container` 创建 Repository、Kubernetes Adapter、Service 和 Handler。`api.RegisterRoutes` 只接收已经组装好的依赖并绑定路由。

`RouteDependencies` 将按领域嵌套：Auth、Application、Runtime/Agent、Delivery、Infrastructure、System。分组只改变 Go 内部结构，不改变路由注册顺序或 Gin middleware 顺序。

## Handler Dependency Rules

每个 Handler 构造函数只能接收：

- 明确的 Repository 接口；
- 已组装的领域 Service；
- 领域声明的窄 Adapter 接口；
- 显式配置值，例如加密密钥、JWT 配置或运行时目录。

禁止新增 `interface{}` 基础设施参数、`*k8s.Client`、`*store.Store` 或在 Handler 中创建 Service/Adapter。现存 `DBAdminHandler` 需要的 GORM 管理能力将先提取为 Repository 接口；若范围过大则以独立 task 明确记录，不允许静默保留 Store。

## Domain Migration

### Auth and Application

Auth Handler 只接收 UserRepository、TemporaryTokenService 与 AuthConfig。Application Handler 只接收 Application repositories、QueryService、Release workflow port 和 KubernetesDependencies。

### Runtime and Agent

Runtime Handler 以 RuntimeManager 接口替代具体 `*runtime.KubernetesManager`，接口覆盖部署、删除、健康检查和外部 Runtime 检查所需方法。Agent Handler 和 AgentOperationHandler 以独立的 Agent Kubernetes ports 替代 `interface{}` client。

### Delivery and Registry

Platform、Mirror、Managed Registry 和 Proxy Handler 只接收各自 Service 与资源/诊断 Adapter。Handler 构造不能在 `nil` Service 时创建替代实例。

### Infrastructure

Domain、Certificate、Cluster DNS、Node Join、Storage、K8s Resource、Terminal 和 Diagnostics Handler 只接收现有或新增的窄接口。Bootstrap 负责从同一个底层 Kubernetes Client 创建并复用这些 Adapter。

### System and Observability

Monitoring、Logging、Alerting 与 System Component Handler 只接收 Query/Component/Automation/Workflow Service 和明确 Adapter；不保留 Handler 内的 Service fallback。

## Test Strategy

每个领域先用 Fake Repository、Fake Service 或 Fake Adapter 编写失败测试，再替换构造函数。测试验证：

- Handler 委托到注入的依赖；
- HTTP 错误码和响应保持不变；
- 请求 Context 到达 Adapter；
- Kubernetes 不可用与超时错误不被吞掉；
- Bootstrap 为相关 Handler 提供同一组已组装实例；
- 路由快照与未知 `/api/*` fallback 行为不变。

## Risks and Mitigations

- 构造函数改动面广：按领域单独提交，先全量搜索调用点，使用编译器发现遗漏。
- 兼容嵌入方：生产路径先迁移；公共旧构造保留到阶段六并标注弃用。
- 行为回归：每领域运行 focused test，随后全量 `go test ./...`、构建和路由快照。
- 接口过宽：接口按 Handler 实际调用的方法定义，避免复用万能 Kubernetes Client。

## Alternatives Considered

- 保持扁平 RouteDependencies：短期简单，但领域所有权不清晰；采用分组结构。
- 让 Handler 直接持有 `*k8s.Client`：实现速度快但违反 Adapter 边界，拒绝。
- 一次性删除所有兼容构造：会破坏嵌入方且难以回滚，延后到阶段六。
