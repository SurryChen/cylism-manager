## Why

`AlertingWorkspace.vue`、`LoggingWorkspace.vue` 和 `DiskGrowthWorkspace.vue` 已开始使用 `useAsyncResource` 管理部分读取，但其安装、设置、静默、通知测试及磁盘筛选仍直接调用通用 `api` 并在组件内构造 REST 路径。日志与磁盘筛选变更可能与旧请求重叠，单一错误状态也会让局部失败干扰已成功加载的状态和结果。

## What Changes

- 扩展 `alerting.js`、`logging.js` 和 `monitoring.js`，让三个监控工作区的读取与变更均由命名领域 API 函数拥有。
- 将告警、日志和磁盘增长的可变筛选读取接入 `useAsyncResource`，所有受管读取转发 `AbortSignal`，并在组件卸载或筛选变更时取消或忽略旧请求。
- 将状态、筛选项、查询结果和 mutation 错误拆分到最小可见区域；刷新或查询失败时保留最近成功的状态、筛选项和结果。
- 为 API 契约、筛选竞态、卸载、局部失败及关键 mutation 失败补充回归测试。

## Capabilities

### New Capabilities

- `frontend-monitoring-workspace-api-boundary`: 告警、日志和磁盘增长工作区的 API 归属、请求生命周期及局部错误处理。

### Modified Capabilities

- 无。

## Non-goals

- 不改变后端 REST 路由、认证、响应格式、告警规则或 Kubernetes 资源行为。
- 不改变现有日志查询 payload、磁盘筛选 query 参数、轮询间隔和用户可见流程。
- 不进行组件拆分、全局状态管理或视觉重构。
- 不迁移 `Monitoring.vue` 之外的其他页面或非监控领域调用。

## Impact

- 受影响前端：`web/src/api/`、三个监控工作区及对应测试。
- 保持现有 HTTP 方法、路径、编码、payload 和解包返回值不变。
