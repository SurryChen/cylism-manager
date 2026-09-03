# Design: Background Lifecycle Composition

## Ownership

`bootstrap.Container` 是常驻任务的唯一组合和所有权边界。它从已创建的
Repository、Service 和 Adapter 组装任务；任务不读取 Gin Context、Handler 或
全局变量。

`cmd/platform/main.go` 只负责读取配置、创建 Container 与 Gin、注册路由、启动
HTTP Server，并在退出信号时按以下顺序关闭：停止接收 HTTP 请求、取消后台
Context、等待后台任务完成。HTTP Server 使用带超时的 `Shutdown`，而不是
`Close`。

## Lifecycle API

新增 `bootstrap.BackgroundConfig`，包含：

- Operation Log 保留天数；
- Platform Reconcile 间隔；
- Registry Proxy Reconcile 间隔；
- System Component Reconcile 间隔。

零值使用明确的安全默认值。`Container.StartBackground(parent, config)` 返回一个
`BackgroundTasks` 控制器。控制器提供 `Stop()` 与 `Wait()`；`Stop` 幂等，`Wait`
在所有任务退出后返回。传入 nil Context 时使用可取消的 Background Context。

每个循环启动后立即执行一次，再以配置间隔执行。一次领域错误只记录并等待下
一轮，不得终止整个任务控制器；Context 取消必须立即停止计时器和新的工作。

## Tasks

| Task | Owner | Work |
| --- | --- | --- |
| Platform release | `platform.ReleaseService` | 调用现有 `ReconcileLatest(ctx)` 恢复或完成最近未完成发布。 |
| Registry proxy | 新 `registry.ProxyReconciler` | 列出保存的代理、读取 Deployment 状态、更新状态，并按现有规则清理缓存。 |
| System component | `system_component.ComponentService` | 调用既有 `Run(ctx, interval)`。 |
| Operation log cleaner | `bootstrap.Container` | 立即清理一次并按小时重试；保留天数非正时不启动。 |

Registry Proxy Reconciler 只声明读取 Deployment、清理缓存和可用性所需的窄
Adapter port。HTTP Handler 保留请求解析和 HTTP 错误映射，改为调用同一个
Reconciler 处理单个状态刷新，不得持有自己的后台循环。

## Failure and Shutdown Semantics

- Kubernetes 不可用时 Platform/Registry/System Component 任务安全跳过或记录，
  不影响 HTTP Server 与其余任务。
- 任务将父 Context 原样传给 Service/Adapter；不使用 `context.Background()` 或
  `context.WithoutCancel()` 逃逸进程生命周期。
- 已在执行的远程操作遵从 Context 取消；`Wait` 不丢弃其结果或遗留 goroutine。
- 同一 Container 多次 `StartBackground` 的行为须明确：先停止并等待旧控制器，
  再创建新控制器，避免重复周期任务。

## Test Strategy

- 用可控间隔与 Fake Task/Service 验证立即执行、取消、Stop 幂等和 Wait。
- 用 Fake Platform/Registry Adapter 验证任务使用同一 Bootstrap Service/Adapter
  实例且 Context 透传。
- 验证 Registry Proxy 状态、缺失资源、诊断错误和到期缓存清理保持现有 Handler
  的语义。
- 验证 `main` 的信号关闭路径调用 HTTP `Shutdown` 并等待后台任务。
