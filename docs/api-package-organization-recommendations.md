# API 包组织评估与建议

评估日期：2026-09-03

## 目的与范围

本文评估当前 `internal/api` 的包边界和文件组织，提出后续可维护性改进建议。它不改变 REST 路径、鉴权模型、Bootstrap 组合边界或业务职责，也不是立即执行的重构任务清单。

当前根包已收敛为路由与依赖契约：`router.go` 定义 `RouteDependencies` 并注册路由，`routes_*.go` 按 HTTP 路由领域划分；Handler 位于领域子包。这个总方向正确，应保持。

## 评估原则

1. 先拆同一 Go package 中的文件，不因目录整齐而过早创建新 package。
2. 新 package 仅用于稳定、独立且可单独测试的能力；Go 子目录会形成新 package，并要求跨边界类型导出。
3. HTTP Handler 只处理绑定、上下文、Service 调用和响应。业务规则、SSH/Kubernetes 调用应下沉到 Service 或 Adapter。
4. 保持 `bootstrap -> api.RouteDependencies -> RegisterRoutes` 单向组合，不在路由层重新构造依赖。
5. 以现有路由快照和领域 Handler 测试保护外部契约。

## 当前结构

```text
internal/api/
  agent/           Runtime Agent 的专用 HTTP API
  application/     项目、环境、应用、发布、模板、Endpoint、集成
  auth/            登录、令牌、中间件
  delivery/        镜像仓库、Registry Mirror/Proxy、Chart、平台发布
  infrastructure/  服务器、Kubernetes、存储、网络、证书、节点、终端
  runtime/         Runtime 生命周期与聊天
  shared/          HTTP 响应、身份等跨 Handler 共享能力
  system/          监控、日志、告警、审计、系统组件、Tailscale、DB 管理
  testdata/        Router 路由清单 golden file，仅供测试
  router.go         依赖契约与总路由注册
  routes_*.go       各领域路由绑定
```

生产代码约 17,000 行。主要压力集中在少数超大 Handler，而非目录数量：

| 包 | 观察 | 结论 |
| --- | --- | --- |
| `agent` | `agent_handler.go` 约 1,100 行 | 需要按 Agent API 资源族拆文件，暂不拆 package |
| `application` | 已按项目、环境、发布、模板、Endpoint 等拆分 | 包边界合理，重点控制集成与运行态文件继续膨胀 |
| `auth` | 仅 Handler 与 Middleware | 规模和职责均合适，无需调整 |
| `delivery` | Registry 相关能力清楚，部分 Handler 约 300-500 行 | 保持 package；先下沉节点 SSH 应用能力 |
| `infrastructure` | 覆盖面广，`k8s_handler.go` 约 1,100 行 | 优先拆同包文件；未来再决定是否按稳定能力拆 package |
| `runtime` | Runtime 与 Chat 两个明确资源族 | 当前合适，随聊天功能增长再拆文件 |
| `shared` | HTTP/身份等小型公共能力 | 保持小且无业务语义，禁止变成万能工具箱 |
| `system` | 监控、告警、日志、审计、系统组件并存 | 保留运维领域总边界，优先拆超大 Handler 的文件 |

## 各包建议

### 根 `api`

保留 `router.go` 中的 `RouteDependencies`，因为它是 `RegisterRoutes` 的输入契约。`routes_*.go` 保持只做 URL 到 Handler 方法的绑定。

不应把 Handler 构造、Service fallback 或 Adapter 创建重新放回根包。`testdata/routes.golden` 是 Router 测试的可读路由基线，不参与生产构建。

### `agent`

`AgentHandler` 共享认证、能力校验和审计状态，直接拆为多个 package 的收益较低。应在同一 `package agent` 下按资源族拆分方法：

```text
agent_handler.go                 构造、认证、能力校验、审计、共享私有方法
agent_workload_handler.go        workload、pod、event、PVC、node、scale
agent_registry_handler.go        registry 状态、诊断、verify、pull check
agent_observability_handler.go   DNS、alert、monitoring
agent_maintenance*.go            保留现有维护检查与清理工作流
```

`agent` 当前依赖 `infrastructure` 的 SSH 能力。SSH 不应长期以 HTTP API package 的形式被其他 API 包复用，应在后续单独下沉到明确的 Adapter 或 Service 端口。

### `application`

现有文件已经与路由资源族一致，属于较好的参考结构。继续遵循以下边界：

- `project`、`environment`、`application`、`release`、`template`、`endpoint` 保持独立。
- `integration_application_handler.go` 只承载集成协议与授权边界；通用应用查询继续通过 Service/Repository 复用。
- `runtime_view.go` 不新增 SSH 或 Kubernetes 编排逻辑；读取组装应留在 Query Service/Adapter。

`runtime_view.go` 目前依赖 `infrastructure`，是需要长期处理的跨 API 包依赖之一。

### `auth`

无需拆分。认证 Handler 和 Gin Middleware 共同构成一个紧凑的 HTTP 边界。令牌签发、密码校验和持久化逻辑继续留在对应 Service/Repository，不将其迁入 `shared`。

### `delivery`

Registry 资源族清晰：镜像仓库、节点镜像加速、受管 OCI、Proxy、Chart、平台发布。维持一个 `delivery` package 即可。

优先改善 `registry_node_mirror_applier.go` 对 `infrastructure` SSH 实现的直接依赖：将“对节点应用 registries 配置”定义为 Service 端口，由 Bootstrap 注入执行器。这样 Delivery 不需要依赖 Infrastructure HTTP 实现。

当 `managed_registry_handler.go` 或 `platform_handler.go` 再明显增长时，先提取请求解析、响应映射或 Workflow 调用文件，不创建额外目录。

### `infrastructure`

这是当前最需要文件级整理的包，但仍不应一次拆为多个 package。建议按下列文件组组织，所有文件暂保持 `package infrastructure`：

```text
server/transport  server、ssh、server terminal、network diagnostics、node join、websocket
network           domain、cert、ingress、cluster DNS、network
storage           storage、PVC CRUD、backup、migration、import
kubernetes        k8s resource handlers、pod terminal、CRD
```

首要任务是拆分 `k8s_handler.go`，保留 Handler 定义、构造函数和共享依赖在主文件，将方法移入：

```text
k8s_namespace_handler.go
k8s_workload_handler.go
k8s_service_handler.go
k8s_configmap_handler.go
k8s_secret_handler.go
k8s_ingress_handler.go
```

PVC 的 CRUD、备份、迁移和导入已经按文件拆开，共用 `StorageHandler` 是合理的聚合，不需要再拆 Handler 实例。Domain 和 Certificate 各自是完整资源聚合，也应先提取同包私有辅助方法，而非建立目录。

`nginx_handler.go` 当前没有路由注册或生产构造调用点。确认没有嵌入方依赖后应删除，不为孤立代码建立额外结构。

### `runtime`

`runtime_handler.go` 与 `chat_handler.go` 的资源边界清楚。随着聊天会话、导出或流式响应持续增长，可按 `chat_session_handler.go`、`chat_message_handler.go` 拆文件，但应仍保留 `package runtime`。Runtime 生命周期与聊天不应回流到 `agent`，两者的调用主体不同。

### `system`

`system` 是运维控制面的总边界，保留该 package 合理。建议按子领域保持文件聚合：

- observability：Monitoring、Alerting、Logging，以及各自 Adapter/Automation；
- administration：Audit、DB Admin、Operation；
- cluster operations：System Component、Tailscale。

`alerting_handler.go`、`logging_handler.go`、`monitoring_handler.go` 已通过 Bootstrap 注入查询与组件 Service。后续仅在这些子领域具备独立 HTTP 契约和独立依赖时再拆 package。

### `shared`

只保留跨领域、无业务语义的 HTTP 规范化能力，例如错误响应、用户身份读取和安全辅助。禁止放入 Application、Registry、Kubernetes、监控等领域规则；出现领域复用时优先放入 Service 或 Adapter 层。

## 依赖方向与治理

当前健康的方向是：

```text
bootstrap -> api (RouteDependencies, routes) -> Handler -> Service/Repository/Adapter
```

需要逐步消除的 API 子包横向依赖包括：

```text
application -> infrastructure  (runtime view)
agent       -> infrastructure  (SSH/maintenance/registry)
delivery    -> infrastructure  (节点 registries 配置)
system      -> agent           (告警自动化)
```

这些依赖当前没有形成循环，因此不是紧急故障。处理方式不是让它们互相复制代码，而是将被复用的执行能力下沉到 Service 接口或 Adapter 包，再由 Bootstrap 组装具体实现。

## 建议顺序

1. 删除已确认无调用的 `infrastructure/nginx_handler.go`，并补充或保留必要测试。
2. 在不改变 package 的前提下拆 `K8sHandler` 与 `AgentHandler` 的文件。
3. 将 Delivery、Agent、Application 对 Infrastructure SSH 能力的横向依赖下沉到端口/Adapter。
4. 评估 Observability 是否需要从 `system` 独立，仅在依赖与发布节奏独立后实施。
5. 每项改动后运行 `go test ./...`、`go build ./...` 和 `npm --prefix web run build`；路由变动需审查 `internal/api/testdata/routes.golden` 的差异。

## 不建议的做法

- 不为每个 Handler 创建一个 Go package。
- 不将 Service、Adapter 的创建迁回路由注册函数。
- 不把 SSH、Kubernetes、Registry 等领域逻辑堆入 `api/shared`。
- 不以移动文件替代依赖下沉；跨 API 包复用 HTTP Handler 实现仍会保留错误的层次方向。
