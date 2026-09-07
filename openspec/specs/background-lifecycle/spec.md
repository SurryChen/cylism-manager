# background-lifecycle Specification

## Purpose
TBD - created by archiving change background-lifecycle-composition. Update Purpose after archive.
## Requirements
### Requirement: Bootstrap owns background task lifecycle

系统 SHALL 由 `bootstrap.Container` 创建、启动、停止并等待所有常驻后台任务。

#### Scenario: Starting the platform

- **WHEN** 平台完成 Container 创建并启动后台任务
- **THEN** Platform Release、Registry Proxy、System Component 和已启用的
  Operation Log Cleaner SHALL 使用同一个由 Bootstrap 组装的依赖图运行

### Requirement: Background tasks respect cancellation

系统 SHALL 将生命周期 Context 传递给后台任务及其远程 Adapter，并在停止时等待
任务退出。

#### Scenario: Receiving a shutdown signal

- **WHEN** 平台接收到退出信号
- **THEN** HTTP Server SHALL 停止接收新请求，后台任务 SHALL 接收取消信号，且
  进程 SHALL 在任务退出后完成关闭

### Requirement: Periodic reconciliation preserves domain semantics

系统 SHALL 周期性恢复 Platform 发布，并更新 Registry Proxy 状态和到期缓存，且
不得改变现有状态字段、缓存清理规则或 HTTP 错误语义。

#### Scenario: Reconciling an expired Registry Proxy cache

- **WHEN** 已就绪的 Registry Proxy 超过配置的缓存清理间隔
- **THEN** 系统 SHALL 调用注入的清理 Adapter，并以现有部署中状态持久化结果
  更新该 Proxy

### Requirement: One task failure does not stop other tasks

系统 SHALL 将单次后台任务错误限制在该轮执行，并在后续间隔重试。

#### Scenario: Kubernetes is temporarily unavailable

- **WHEN** 某个 Reconcile Adapter 返回 Kubernetes 不可用错误
- **THEN** 该任务 SHALL 记录或安全跳过本轮，其他后台任务 SHALL 继续运行

