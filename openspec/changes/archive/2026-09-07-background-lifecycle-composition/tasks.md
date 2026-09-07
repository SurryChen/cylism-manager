# Tasks

## 1. Lifecycle contract

- [x] 1.1 添加 `BackgroundTasks` 的失败测试：立即执行、取消、Stop 幂等和 Wait。
- [x] 1.2 实现 `BackgroundConfig`、控制器与 Container 单一启动/停止边界。
- [x] 1.3 为 Operation Log Cleaner 改为立即执行加周期循环，保留禁用语义。

## 2. Domain reconcilers

- [x] 2.1 为 Platform `ReconcileLatest` 添加可控周期任务测试与 Context 透传断言。
- [x] 2.2 先为 Registry Proxy Reconcile 添加 Fake Repository/Adapter 测试，覆盖 ready、missing、诊断失败和缓存到期清理。
- [x] 2.3 提取 `registry.ProxyReconciler` 的窄 Adapter port，迁移 Handler 的状态刷新复用该 Reconciler。
- [x] 2.4 由 Bootstrap 组装并启动 Platform 与 Registry Proxy 周期任务。
- [x] 2.5 保持 System Component 单例 Service 的既有循环，并接入统一控制器。

## 3. Process shutdown

- [x] 3.1 为平台进程关闭编排添加测试，验证 `Shutdown`、后台停止和等待顺序。
- [x] 3.2 将 `main.go` 改为使用统一后台控制器与带超时的 HTTP 优雅关闭。

## 4. Verification

- [x] 4.1 每个 task 完成后执行旧调用点搜索、相关包测试和 `git diff --check`。
- [x] 4.2 执行宿主机 `go test ./...`、`go build ./...` 和 `npm --prefix web run build`。
- [x] 4.3 执行 `openspec validate background-lifecycle-composition --strict`，呈报结果并等待用户确认后归档。
