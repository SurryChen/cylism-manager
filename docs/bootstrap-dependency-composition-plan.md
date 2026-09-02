# Bootstrap 依赖组装层迁移计划

状态：阶段一已完成，阶段二进行中（Cluster/System Component/Monitoring/Logging/Alerting Adapter 已迁移，2026-09-02）

本文档规划将应用初始化、第三方依赖接入、Repository/Service/Handler 创建从 `cmd/platform` 和 `internal/api/router.go` 逐步迁移到 `internal/bootstrap`。

目标是在不改变 REST API、WebSocket、数据库结构、Kubernetes 行为和前端工作流的前提下，建立清晰的依赖方向：

```text
cmd/platform
    ↓
internal/bootstrap
    ├── Store / Repository
    ├── Kubernetes Client / Adapter
    ├── Service
    └── API Handler
            ↓
        internal/api
```

## 一、设计原则

### 1. Service 定义自己需要的接口

Service 不直接依赖完整的 `*k8s.Client`，而是根据用例定义最小能力接口。

```go
type NodeReader interface {
    ListNodes(ctx context.Context) ([]NodeInfo, error)
}

type DeploymentReader interface {
    GetDeployment(ctx context.Context, namespace, name string) (*DeploymentInfo, error)
}
```

一个 Service 可以依赖多个接口，也可以在包内组合接口：

```go
type clusterResources interface {
    NodeReader
    DeploymentReader
}
```

禁止创建包含所有 Kubernetes 能力的万能 `KubernetesClient` 接口。

### 2. Adapter 实现接口，Bootstrap 负责接入

```text
Service 定义接口
    ↓
internal/k8s Adapter 实现接口
    ↓
internal/bootstrap 创建并注入 Adapter
```

同一个 Adapter 实例可以实现多个窄接口，并被多个 Service 复用：

```go
adapter := k8s.NewResourceAdapter(client)

clusterService := cluster.NewService(repo, adapter)
monitoringService := monitoring.NewService(adapter)
systemService := systemcomponent.NewService(adapter)
```

每个 Service 仍然只通过自身声明的接口访问 Adapter。

### 3. API 只负责 HTTP 边界

API 层只负责：

- 请求绑定；
- 鉴权上下文读取；
- 调用 Service；
- 将结果映射为 HTTP 响应；
- 注册路由。

API 不负责创建 Store、Kubernetes Client、Adapter 或业务 Service。

### 4. 依赖组装集中在 Bootstrap

`internal/bootstrap` 是应用组合根（Composition Root），负责：

- 读取并传递进程配置；
- 创建 Store；
- 创建 Kubernetes Client；
- 创建各领域 Adapter；
- 创建 Repository、Service 和 Handler；
- 启动后台 Reconcile/清理任务。

## 二、目标目录结构

```text
cmd/platform/
  main.go                         # 进程入口，只负责配置、启动和退出

internal/
  bootstrap/
    container.go                  # 总依赖容器
    config.go                     # 进程配置（可选，后续抽取）
    kubernetes.go                 # K8s Client 和 Adapter 创建
    repositories.go               # Repository 创建
    services.go                   # Service 创建
    handlers.go                   # Handler 创建
    background.go                 # 后台任务生命周期
    container_test.go

  api/
    router.go                     # 接收已组装依赖并注册路由
    routes_*.go                   # 路由绑定
    auth/
    application/
    runtime/
    agent/
    delivery/
    infrastructure/
    system/
    shared/
    infrastructure/transport/     # WebSocket 等通用传输（后续）

  service/
    cluster/
      service.go
      ports.go                    # NodeReader、NodeLifecycle 等接口
      types.go                    # Cluster 领域 DTO
    application/
      service.go
      query.go
      ports.go
    registry/
      service.go
      ports.go
    storage/
      service.go
      ports.go
    network/
      service.go
      ports.go
    platform/
      service.go
      ports.go
    observability/
      alerting/
        service.go
        ports.go
      logging/
        service.go
        ports.go
      monitoring/
        service.go
        ports.go
    system_component/
      service.go
      ports.go

  k8s/
    client.go                    # 底层 Kubernetes Client
    node_adapter.go              # 节点读取和生命周期
    workload_adapter.go          # Deployment/StatefulSet/Pod
    storage_adapter.go           # PVC
    network_adapter.go           # Ingress/DNS/Certificate
    observability_adapter.go     # Loki/VM/Alertmanager
    system_component_adapter.go  # Helm/Deployment
    registry_adapter.go          # Registry 资源
    namespace.go
    config.go
    ingress.go
    reconciler_*.go

  repository/
    application_repository.go
    runtime_repository.go
    infrastructure_repository.go
    observability_repository.go
    registry_repository.go

  store/
    db.go
    migrations.go
    application_repository.go
    runtime_repository.go
    infrastructure_repository.go
    observability_repository.go
    registry_repository.go

  model/
    application.go
    runtime.go
    infrastructure.go
    observability.go
    registry.go
    auth.go
```

## 三、当前状态

阶段一已经完成：

- 新增 `internal/bootstrap/container.go`；
- 新增 `internal/bootstrap/kubernetes.go`；
- 新增 `internal/bootstrap/repositories.go`；
- 新增 `internal/bootstrap/services.go`；
- 新增 `internal/bootstrap/handlers.go`；
- `cmd/platform/main.go` 已通过 `bootstrap.NewContainer` 创建 Store 和 Kubernetes Client；
- `Container.RegisterRoutes` 已成为新的启动入口；
- Bootstrap 容器测试、Go 编译检查和 `go build ./...` 已通过。

阶段二已完成的部分：

- `KubernetesAdapters` 已在 `internal/bootstrap/kubernetes.go` 定义；
- Cluster `NodeAdapter` 已由 Bootstrap 创建并通过 `RegisterRoutes` 显式注入；
- `SystemComponentAdapter` 已由 Bootstrap 创建并注入 Handler 及后台 Reconcile；
- Monitoring、Logging、Alerting 的查询、就绪、组件和资源读取 Adapter 已由 Bootstrap 创建并注入；
- `internal/api/cluster_adapter.go` 已删除，Router 不再创建这两个 Adapter。

当前仍存在的过渡逻辑：

- `api.RegisterRoutes` 仍创建大部分 Service 和 Handler；
- `api/application`、`api/runtime` 的包级 `K8s` 变量仍待迁移；
- Network、Storage、Registry Adapter 仍在 Router 中组装，等待对应窄接口完成；
- `api.RegisterRoutes` 仍创建大部分 Service 和 Handler，并启动 Platform/Registry Proxy 的历史 Reconcile 任务；
- `bootstrap/services.go`、`handlers.go` 目前是后续迁移落点。

本轮已完成的阶段一收尾：

- `main.go` 不再直接创建 System Component Handler 或操作日志清理循环；
- `Container.StartBackground` 统一启动 System Component Reconcile 和操作日志清理，并接收 Context 生命周期；
- `api.RegisterRoutes` 改为显式接收 `*k8s.Client`，不再依赖根包 `api.K8s` 全局变量；
- `clusterNodeAdapter` 改为接收显式 Client 参数，保留为临时适配函数，不再读取全局 API 状态；
- 路由快照测试和全量编译调用点已同步更新。

## 四、分阶段实施计划

### 阶段 1：Bootstrap 基础骨架

目标：建立稳定的组合根，不改变业务行为。

任务：

- 定义 `bootstrap.Config`；
- 定义 `bootstrap.Container`；
- 集中创建 Store；
- 集中创建 Kubernetes Client；
- 将认证配置放入 Container；
- 将 `main.go` 改为使用 Container；
- 增加 Container 单测。

验收：

- `go build ./...` 通过；
- Container 可以在 K8s 不可用时降级启动；
- 数据库和 JWT 配置行为不变。

### 阶段 2：Kubernetes Adapter 组装迁移

目标：移除 API Router 中的 Kubernetes Adapter 创建逻辑。

优先迁移：

1. `cluster.NodeAdapter`
2. `SystemComponentAdapter`
3. Monitoring 的 Metrics、PVC Consumer 和 Component Adapter
4. Logging 的 Filter、Component 和 readiness Adapter
5. Alerting 的 Alertmanager、Secret、Component 和 readiness Adapter
6. Network、Storage、Registry Adapter

Bootstrap 示例：

```go
func buildKubernetesAdapters(client *k8s.Client) KubernetesAdapters {
    if client == nil {
        return KubernetesAdapters{}
    }
    return KubernetesAdapters{
        Nodes:       k8s.NewNodeAdapter(client),
        Workloads:   k8s.NewWorkloadAdapter(client),
        Monitoring:  k8s.NewMonitoringAdapter(client),
        Logging:     k8s.NewLoggingAdapter(client),
        Alerting:    k8s.NewAlertingAdapter(client),
        Components:  k8s.NewSystemComponentAdapter(client),
    }
}
```

要求：

- Adapter 创建只发生在 Bootstrap；
- 同一底层 Adapter 可实现多个窄接口；
- Service 不读取全局 `api.K8s`；
- 所有远程方法接收 `context.Context`。

### 阶段 3：Repository 和 Service 组装迁移

目标：让 `bootstrap/services.go` 成为所有业务 Service 的唯一创建位置。

迁移顺序：

1. Application/Runtime
2. Cluster/Network/Storage
3. Registry/Platform
4. Monitoring/Logging/Alerting
5. System Component

示例：

```go
func buildServices(repos Repositories, adapters KubernetesAdapters) Services {
    return Services{
        Cluster:     cluster.NewService(repos.Cluster, adapters.Nodes),
        Application: application.NewService(repos.Application, adapters.Workloads),
        Monitoring:  monitoring.NewService(repos.Observability, adapters.Monitoring),
    }
}
```

每个 Service 的构造函数应：

- 接收 Repository 接口；
- 接收窄 Adapter 接口；
- 不接收 `*store.Store`；
- 不接收完整 `*k8s.Client`；
- 不读取全局变量。

### 阶段 4：Handler 组装迁移

目标：让 `bootstrap/handlers.go` 创建全部 API Handler。

Handler 只接收：

- Repository 接口；
- Service；
- Adapter 接口；
- 加密/JWT 等明确配置。

迁移领域：

- Auth；
- Application；
- Runtime/Agent；
- Delivery/Registry；
- Infrastructure；
- System/Observability。

完成后 API Router 改为接收依赖结构：

```go
type Dependencies struct {
    Auth          *authapi.AuthHandler
    Application   *applicationapi.ApplicationHandler
    Runtime       *runtimeapi.RuntimeHandler
    Registry      *delivery.RegistryHandlers
    Infrastructure *infrastructureapi.Handlers
    System        *systemapi.Handlers
}

func RegisterRoutes(r *gin.Engine, deps Dependencies) {
    registerPublicRoutes(r, deps.Agent.Artifact, deps.Agent.Handler)
    registerAuthRoutes(r, deps.Auth, deps.AuthConfig)
    registerApplicationRoutes(r, deps.Application, ...)
}
```

Router 不再创建任何业务依赖。

### 阶段 5：后台任务与生命周期迁移

目标：统一后台任务的启动和关闭边界。

迁移内容：

- Platform Reconcile；
- Registry Proxy Reconcile；
- System Component Reconcile；
- Operation Log Cleaner；
- 其他定时任务。

建议使用 Context 控制生命周期：

```go
func (c *Container) StartBackground(ctx context.Context) []func() {
    // 启动任务并返回停止函数
}
```

`main.go` 只负责：

1. 读取配置；
2. 创建 Container；
3. 创建 Gin；
4. 注册路由；
5. 启动 HTTP Server；
6. 处理退出信号。

### 阶段 6：删除过渡依赖

完成前述迁移并通过验证后，删除：

- `api.K8s` 全局变量；
- `cluster_adapter.go`；
- Router 内的 Kubernetes Adapter 组装；
- Router 内的 Service/Handler 创建；
- `main.go` 中重复的 Handler 创建；
- 无调用方的兼容构造函数；
- Bootstrap 迁移期间产生的临时包装。

`cluster_adapter.go` 删除后的目标写法：

```go
nodes := bootstrap.BuildNodeAdapter(k8sClient)
clusterService := cluster.NewService(clusterRepo, nodes)
```

## 五、接口和 Adapter 复用规则

### 可以复用的情况

- 方法语义完全一致；
- 参数、返回值和错误语义一致；
- 至少两个稳定消费者；
- 共享不会导致 Service 暴露无关能力。

### 不应强行复用的情况

- 只有方法名称相同但语义不同；
- 一个领域需要管理权限，另一个只读；
- 超时、重试或错误处理策略不同；
- 为了复用而引入跨领域 DTO；
- 需要把几十种 Kubernetes 资源放进一个接口。

### 推荐模式

```text
cluster.Service
  ├── NodeReader
  ├── NodeLifecycle
  └── DeploymentReader

monitoring.Service
  ├── NodeReader
  ├── PodReader
  └── MetricsReader

system_component.Service
  ├── DeploymentReader
  ├── NodeReader
  └── ComponentStatusReader
```

同一个 Adapter 可以实现多个接口，但每个 Service 只声明自己使用的接口。

## 六、依赖方向约束

```text
cmd/platform
    ↓
bootstrap
    ├── api
    ├── service
    ├── repository
    └── k8s

api → service/application → repository/k8s → store/model
service → repository/model
k8s → Kubernetes SDK/model
store → model
```

禁止：

- `service` 导入 `api`；
- `service` 读取 `api.K8s`；
- `api` 反向依赖 Bootstrap 内部实现；
- Handler 创建底层 Kubernetes Adapter；
- Service 直接操作 GORM；
- Service 接收包含全部能力的万能 Client。

## 七、测试策略

### Bootstrap 测试

- Store 初始化成功；
- K8s 不可用时 Container 正常创建；
- Auth 配置正确传递；
- Adapter 创建结果符合预期；
- 后台任务可以启动和停止。

### Service 测试

- 使用 Fake Repository；
- 使用 Fake Adapter；
- 验证 context 透传；
- 验证 K8s 不可用和超时错误；
- 验证跨 Service 复用同一 Adapter 不产生状态污染。

### Handler 测试

- 验证 Handler 委托到 Service/Fake；
- 验证鉴权失败；
- 验证参数错误；
- 验证 K8s 不可用响应；
- 验证错误码和 HTTP 状态码保持不变。

### 路由测试

- 保留完整路由快照；
- 验证未知 `/api/*` 路径不会落入 SPA fallback；
- 验证路由绑定的 Handler 来自 Bootstrap 依赖结构。

## 八、每阶段验证命令

```bash
GOCACHE=/tmp/cylism-go-build GOWORK=off GOPROXY=off go test ./...
GOCACHE=/tmp/cylism-go-build GOWORK=off GOPROXY=off go build ./...
git diff --check
```

前端：

```bash
source ~/.nvm/nvm.sh
nvm use 24
npm --prefix web run build
```

如果测试因沙箱禁止 `httptest` IPv6 监听而失败，应在宿主机环境重新执行，并区分环境限制和代码失败。

## 九、迁移门禁和回滚边界

每迁移一个领域必须：

1. 使用 `rg` 搜索旧构造函数、全局变量和旧 Adapter 调用点；
2. 先补充或移动 Fake 测试；
3. 执行领域测试；
4. 执行全量 Go 测试和构建；
5. 检查路由快照没有变化；
6. 检查 `git diff --check`；
7. 独立提交，确保可回滚。

不允许在一次变更中同时重写业务逻辑、数据库结构和依赖组装。每次迁移只改变对象创建位置和依赖传递方式。

## 十、下一步执行项

下一步优先完成阶段 2 的第一小步：

1. 在 `bootstrap/kubernetes.go` 中创建 `cluster.NodeAdapter`；
2. 修改 `cluster.NewService` 的组装调用，直接接收该 Adapter；
3. 删除 `clusterNodeAdapter()` 的生产调用；
4. 保留行为一致性测试；
5. 确认通过后再迁移 System Component、Monitoring 和 Logging Adapter。

完成第一小步后，再继续阶段 3 的 Service 组装迁移，避免一次性移动所有 Handler 导致难以定位问题。
