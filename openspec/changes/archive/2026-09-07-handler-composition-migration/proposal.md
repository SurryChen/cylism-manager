# Handler Composition Migration

## Why

Bootstrap 已接管路由依赖的主组装路径，但多个 API Handler 仍保留宽泛的 `interface{}` Kubernetes 参数、具体运行时 Manager、`*store.Store` 依赖或自行创建领域 Service 的兼容构造。这样会使 HTTP 边界继续承担对象图组装责任，并使单元测试难以用窄 Fake 验证。

## What Changes

- 让 `internal/bootstrap/handlers.go` 成为所有生产 API Handler 的唯一创建位置。
- 将 Runtime、Agent 和 Infrastructure Handler 的宽泛或具体基础设施依赖替换为领域定义的窄接口。
- 将 Handler 的持久化依赖收紧为 Repository 接口；保留 Store 仅限当前无法以已有 Repository 表达的数据库管理能力。
- 删除 API 生产路径中的 Service/Adapter fallback；测试改用显式 Service、Adapter 和 Repository Fake。
- 将 RouteDependencies 按领域分组，同时保留所有既有路由、鉴权和 HTTP 响应契约。

## Capabilities

### Added Capabilities

- `handler-composition`: HTTP Handler 通过 Bootstrap 提供的显式依赖运行，不在 API 层创建领域 Service 或 Kubernetes Adapter。

### Modified Capabilities

- `user-auth`: Auth Handler 的临时令牌工作流由 Bootstrap 注入，登录与令牌响应保持不变。
- `infrastructure-api-services`: 基础设施 Handler 继续保持 REST 契约，但依赖改为 Repository、Service 和窄 Adapter。

## Non-goals

- 不修改 REST 路径、HTTP 方法、鉴权规则、响应包装、错误码或数据库结构。
- 不改变 Kubernetes 资源渲染、SSH 操作或后台任务业务语义。
- 不在此变更中实施阶段五后台任务生命周期迁移。
- 不删除公共兼容构造函数；只有确认无生产或嵌入调用方后，才在阶段六集中删除。

## Impact

- 受影响包：`internal/bootstrap`、`internal/api/{auth,application,runtime,agent,delivery,infrastructure,system}` 和相应领域 Service。
- 需要更新 Handler 单测以显式注入 Fake Repository、Service、Adapter。
- 路由快照和前端 API 消费无需变更。
