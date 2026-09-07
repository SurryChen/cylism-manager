# Tasks

## 1. Dependency graph and route contract

- [x] 1.1 添加 Bootstrap 失败测试，验证分组 RouteDependencies 包含所有现有 Handler，Router 只绑定依赖。
- [x] 1.2 将 RouteDependencies 按 Auth、Application、Runtime/Agent、Delivery、Infrastructure、System 分组；更新路由注册和快照测试，保持路由不变。
- [x] 1.3 搜索并记录所有 Handler 构造函数、`interface{}` client、`*store.Store` 和生产 fallback 调用点。

## 2. Auth, Application, Runtime and Agent

- [x] 2.1 先添加 Auth/Application 构造测试，改为只接受显式 Repository、Service 与配置。
- [x] 2.2 为 Runtime 定义最小 Manager port，替换具体 KubernetesManager 和请求内 fallback；补充 Context 与不可用测试。
- [x] 2.3 为 Agent/AgentOperation 定义 Kubernetes ports，替换 `interface{}` client；补充 Fake Adapter 与错误映射测试。

## 3. Delivery and Registry

- [x] 3.1 为 Platform、Mirror、Managed Registry、Proxy Handler 补充显式 Service 注入测试。
- [x] 3.2 切换所有生产调用点并移除 Handler 内 nil-Service fallback。
- [x] 3.3 验证 Registry 资源/诊断 Adapter 由 Bootstrap 复用且 Context 透传。

## 4. Infrastructure

- [x] 4.1 为 Cluster DNS、Domain、Certificate、Node Join 定义并注入窄 Adapter；删除 `interface{}` client 参数。
- [x] 4.2 收紧 Storage、K8s Resource、Terminal、Diagnostics 和 DBAdmin 的 Repository/Adapter 依赖。
- [x] 4.3 为每个迁移领域补充 Handler Fake、错误码、K8s 不可用和 Context 透传测试。

## 5. System and Observability

- [x] 5.1 删除 Monitoring、Logging、Alerting、System Component Handler 内的生产 Service fallback。
- [x] 5.2 由 Bootstrap 注入 Query、Component、Automation 和 ComponentService；验证复用同一实例。

## 6. Verification

- [x] 6.1 每个领域完成后执行 `rg` 搜索旧构造和调用点、focused Go 测试、路由快照与 `git diff --check`。
- [x] 6.2 执行宿主机 `go test ./...`、`go build ./...` 和 `npm --prefix web run build`。
- [x] 6.3 执行 `openspec validate handler-composition-migration --strict`，呈报结果并等待用户确认后归档。
