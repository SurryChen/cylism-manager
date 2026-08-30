# Cylism Manager Internal Architecture Optimization Plan

状态：阶段 0 已完成（2026-08-30），阶段 1～6 待执行

本文档针对 `internal/` 目录的可维护性和复用能力进行优化规划。目标是逐步收窄模块职责、减少隐式依赖和重复逻辑，同时保持现有 REST API、数据库结构、Kubernetes 资源行为和前端工作流不变。

## 1. 现状基线

当前 `internal/` 已经具备 API、Service、K8s Adapter、Store 和 Domain Model 的基本分层，但领域边界仍不均衡：

| 位置 | 生产代码规模 | 主要问题 |
|---|---:|---|
| `internal/api` 根包 | 约 5592 行 | 应用、运行时和通用路由仍集中在根包 |
| `internal/api/application_handler.go` | 2073 行 | 同时处理项目、环境、应用、发布、模板和 Endpoint |
| `internal/api/router.go` | 527 行 | 路由注册和依赖组装耦合在一个函数 |
| `internal/api/infrastructure/k8s_handler.go` | 1103 行 | Namespace、Workload、Service、ConfigMap、Secret、Ingress 混在一个 Handler |
| `internal/store/store.go` | 1923 行 | 约 190 个跨领域持久化方法集中在一个 Store |
| `internal/k8s` | 约 7300 行 | 每个文件虽按功能拆分，但 Adapter、Reconcile、状态查询仍经常混合 |
| `internal/application/spec.go` | 955 行 | 发布 Spec、校验、默认值和转换逻辑集中 |
| `internal/api/system/alerting_handler.go` | 约 978 行 | HTTP、Alertmanager、SMTP 和自动化职责混合 |

已经完成的领域迁移包：

```text
internal/api/agent/
internal/api/delivery/
internal/api/infrastructure/
internal/api/system/
```

这些包作为后续拆分的参考边界，不再回退到 API 根包。

## 2. 优化目标

1. 一个 Handler 只负责一个业务聚合或一个基础设施资源族。
2. Handler 只做请求绑定、鉴权上下文提取、Service 调用和响应映射。
3. 业务规则进入 `internal/service` 或 `internal/application`，持久化进入 Repository/Store Adapter，Kubernetes 操作进入 `internal/k8s` Adapter/Reconciler。
4. 消除跨领域包复用内部辅助函数的情况，公共能力进入明确的 shared/security 包。
5. 将根包 `internal/api` 收敛为认证、应用组装和少量尚未迁移的领域入口。
6. 每次迁移都可以独立验证、独立提交和回滚。

## 3. 目标结构

```text
internal/
  api/
    auth/
    application/
      project_handler.go
      environment_handler.go
      application_handler.go
      release_handler.go
      template_handler.go
      endpoint_handler.go
      runtime_view.go
    agent/
    delivery/
    infrastructure/
    system/
    shared/
      http.go
      identity.go
      sanitize.go
  application/
    spec.go
    validation.go
    normalization.go
    release_workflow.go
    kubernetes.go
  service/
    application/
    observability/
    cluster/
    network/
    platform/
    registry/
    storage/
  repository/
    runtime_repository.go
    application_repository.go
    release_repository.go
    infrastructure_repository.go
    observability_repository.go
  k8s/
    workload.go
    namespace.go
    config.go
    ingress.go
    system_components.go
    reconciler_*.go
  store/
    db.go
    migrations.go
    repositories.go
  model/
    application.go
    runtime.go
    infrastructure.go
    registry.go
    observability.go
```

这里的文件拆分优先于 Go package 拆分。同一领域内部先保持 package 不变，只有出现清晰的依赖边界和独立测试价值时才新建 package，避免为了目录数量增加而增加适配代码。

## 4. 分阶段实施

### 阶段 0：基线和低风险清理（已完成）

目标：建立可比较的基线，先处理不涉及结构变化的明确问题。

任务：

- 修正 `internal/api/system/dependencies.go` 中错误的 `k8sClient 集群未连接` 文案。
- 新增 `internal/api/shared`，统一 `getUserID`、`k8sUnavailable` 和 ID 解析的实现归属；所有调用点已直接使用 shared 函数，原有兼容 helper 已删除。
- 核对现有领域 API 测试和 Router 路由清单，后续按领域迁移继续补充成功、鉴权失败、Kubernetes 不可用和 Store 错误的路由级回归测试。
- 记录 `internal/api` 复杂度、路由分组和调用点清单。
- 记录 `go test ./...`、`go build ./...` 和前端构建结果，详见 `docs/internal-architecture-stage0-baseline.md`。

验收：无 API 行为变化，完整测试和构建通过。

### 阶段 1：Application API 拆分

目标：处理当前最大的 `ApplicationHandler`，这是降低复杂度和提高复用率的最高优先级。

拆分范围：

- 项目和环境：`project_handler.go`、`environment_handler.go`
- 应用生命周期：`application_handler.go`
- 发布、重启、重试和回滚：`release_handler.go`
- Deployment Template：`template_handler.go`
- Application Endpoint：`endpoint_handler.go`
- 工作台和运行状态查询：`runtime_view.go`

要求：

- 保持现有 `NewApplicationHandler` 的外部行为，先使用同一个内部依赖结构。
- 将重复的应用查询、环境解析和运行状态组装提取为小型只读 Service。
- 发布流程继续复用 `application.ReleaseWorkflow`，不在新 Handler 中复制状态机。
- 每个拆分文件的测试与源文件同目录，先移动测试再调整实现。

验收指标：

- 单个 Application HTTP Handler 文件原则上不超过 800 行。
- 发布和 Endpoint 逻辑不再直接复制项目/环境查询代码。
- 现有应用创建、发布、重启、回滚、模板和 Endpoint 页面回归通过。

### 阶段 2：Store 和 Repository 分域

目标：消除 `Store` 作为全系统万能依赖的情况。

顺序：

1. 先为 Runtime、Application、Release、Infrastructure、Observability 定义最小 Repository 接口。
2. 用现有 `Store` 实现这些接口，保持数据库实现不变。
3. 逐个 Service 和 Handler 改用接口，使用 `rg` 检查旧 Store 调用点。
4. 将查询和事务实现物理移动到 Repository 文件。
5. 最后把 `store.go` 收敛为数据库初始化、迁移和兼容入口。

不做的事情：

- 不一次性重写所有 Store 方法。
- 不在 Repository 中放业务规则或 Kubernetes 调用。
- 不为每个简单字段访问创建没有复用价值的抽象。

验收：Service 单测可以使用内存 Repository 或 Stub，不需要启动完整 API；事务、唯一性和并发状态更新回归通过。

### 阶段 3：Kubernetes Adapter 和 Reconciler 整理

目标：降低 `internal/k8s` 中单个 Client 的职责密度。

拆分方向：

- 通用资源：Namespace、ConfigMap、Secret、Service、Ingress。
- Workload：Deployment、StatefulSet、DaemonSet、Pod 状态和日志。
- 平台组件：CoreDNS、Traefik、Metrics Server、Loki、VictoriaMetrics、Alertmanager。
- Reconciler：资源 Apply、删除、等待就绪和恢复逻辑。
- Query/Status：只读状态查询和摘要转换。

实施原则：

- 先在 `internal/k8s` 内按文件拆分，避免过早拆成多个 package。
- 新增接口只暴露业务需要的方法，不把完整 `k8s.Client` 传入 Service。
- 所有远程操作必须接受 context，并保留超时和错误包装。
- Reconcile 只负责目标状态收敛，不负责 HTTP 响应格式。

验收：系统组件、Registry、PVC、应用发布的 K8s 行为不变；Service 不再直接拼接 Kubernetes 资源对象。

### 阶段 4：System 和 Observability Service 化

目标：继续压缩 system Handler 的业务代码。

重点：

- Alertmanager 查询、静默、通知和 SMTP 移入 `service/observability/alerting`。
- Loki 查询表达式校验、结果归一化和过滤移入 `service/observability/logging`。
- VictoriaMetrics 查询、时间范围和磁盘增长分析移入 `service/observability/monitoring`。
- 系统组件的白名单、配置检测和超时状态移入 `service/system_component`。
- Tailscale 的主机命令执行封装成可测试 Adapter，Handler 不直接调用 `os/exec`。
- 脱敏和截断放入 `internal/api/shared/sanitize` 或独立安全包，禁止 system 依赖 agent 包内部工具。

验收：system Handler 主要由 DTO、鉴权、Service 调用和响应映射组成；告警、日志、监控单测可以注入 Fake Client。

### 阶段 5：路由组装和公共 API 能力收敛

目标：降低 Router 和根包的耦合。

任务：

- 将 `RegisterRoutes` 拆成按领域的注册函数，但保留唯一的依赖组装入口。
- 统一 API 错误响应、身份读取、分页和参数解析。
- 清理历史兼容构造函数和不再使用的可变参数构造函数。
- 保留根包中认证、启动组装和尚未迁移的模块，禁止重新放入大型业务 Handler。

验收：所有路由路径通过路由快照或集成测试核对；前端请求不落入 SPA fallback。

### 阶段 6：Model 和目录收尾

目标：改善导航体验，不改变模型包的公开类型归属。

- 将 `model/models.go` 按领域拆分为多个文件，暂不拆 package。
- 将 `application/spec.go` 拆为 Spec、校验、默认值和序列化文件。
- 更新架构文档、依赖图和开发流程说明。
- 删除迁移期间的临时 Adapter、重复测试辅助函数和死代码。

## 5. 每个模块的执行门禁

每个模块必须独立完成以下步骤：

1. 记录迁移前测试结果和路由清单。
2. 用 `rg` 搜索旧构造函数、旧方法和全局变量调用点。
3. 先补充或移动测试，再进行实现调整。
4. `gofmt -w` 所有变更的 Go 文件。
5. 执行 `go test ./...`、`go build ./...` 和前端构建。
6. 对涉及页面的功能验证列表、详情、创建、更新、删除、错误和空状态。
7. 检查 `git diff --check`，单模块独立提交。

## 6. 量化检查项

每个阶段结束时检查：

- 最大生产 Handler 文件是否继续下降。
- `Store` 直接出现在 Handler 中的调用数量是否下降。
- Service 是否依赖 API 包。
- system 是否依赖 agent API 包。
- 包级可变 Kubernetes 全局状态是否减少。
- 重复的身份、错误、解析和脱敏辅助函数是否减少。
- 完整测试、构建和前端构建是否通过。

建议目标不是单纯减少总行数，而是减少职责耦合和重复实现。新增的接口、测试或 Adapter 必须能对应一个明确的边界、替换点或回归风险；否则不应为了“分层”而增加代码。

## 7. 风险与回滚

- API 拆分可能改变 Gin 路由绑定：迁移前后保存路由路径清单并做集成测试。
- Repository 替换可能改变事务边界：先保留原 Store 实现，再逐个切换调用方。
- K8s Adapter 拆分可能改变超时和错误包装：所有远程调用保留原有 context、超时和中文错误码。
- 公共辅助函数合并可能引入包循环：优先放入低层 shared 包，禁止 shared 反向依赖 Handler。
- 每个阶段只处理一个领域，发现回归时只回滚当前模块提交。

## 8. 推荐执行顺序

```text
阶段 0  基线和低风险清理
  -> 阶段 1  Application API 拆分
  -> 阶段 2  Store / Repository 分域
  -> 阶段 3  Kubernetes Adapter 整理
  -> 阶段 4  Observability Service 化
  -> 阶段 5  Router 和公共 API 收敛
  -> 阶段 6  Model 与目录收尾
```

优先开始阶段 0 和阶段 1。它们能最快降低当前代码阅读和修改成本，并为后续 Repository、Kubernetes Adapter 和 Observability 拆分提供稳定边界。
