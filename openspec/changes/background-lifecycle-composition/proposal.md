# Background Lifecycle Composition

## Why

阶段四已将 HTTP Handler 的生产组装集中到 Bootstrap，但后台循环仍只有
System Component Reconcile 与 Operation Log Cleaner，且由 `main.go` 直接持有
取消函数。Platform 发布恢复和 Registry Proxy 状态/缓存维护没有统一的常驻
调度边界。任务启动、停止和等待缺少一个可测试的所有者。

## What Changes

- 让 `bootstrap.Container` 创建和拥有全部常驻后台任务。
- 引入可等待的后台任务控制器，统一 Context 取消、停止和 `Wait`。
- 周期性执行 Platform Release 恢复、Registry Proxy Reconcile、System Component
  Reconcile 和 Operation Log Cleaner。
- 将 Registry Proxy 的状态刷新与定期缓存清理提取为无 HTTP 依赖的领域
  Reconciler。
- 将 `cmd/platform/main.go` 收敛为配置、HTTP Server 启动和优雅退出编排。

## Non-goals

- 不迁移请求触发的异步发布、PVC 迁移/导入/备份或 WebSocket 生命周期。
- 不改变现有 REST 路径、响应、Registry Proxy 缓存清理规则或 Platform 发布状态机。
- 不引入独立任务队列、持久化调度器或分布式锁。

## Impact

- 受影响包：`cmd/platform`、`internal/bootstrap`、`internal/service/platform`、
  `internal/service/registry` 和 Registry Proxy HTTP Handler。
- 新增后台任务与关闭行为测试，验证启动即执行、间隔执行、取消、等待以及
  Context 透传。
