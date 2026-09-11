## Why

`web/src/components` 目前同时放置跨页面通用组件和只服务单个页面的业务工作区组件，导致目录语义变弱：维护者需要在全局组件目录中寻找监控、Agent、服务器终端和 Pod 终端逻辑。现在按已有 `views` 领域收口页面专属组件，可以让组件归属更清晰，同时不改变用户可见行为。

## What Changes

- 将监控专属组件迁移到 `web/src/views/monitoring/`：
  - `AlertingWorkspace.vue`
  - `LoggingWorkspace.vue`
  - `DiskGrowthWorkspace.vue`
  - `MetricTrendChart.vue`
- 将 Runtime 专属 `ChatDrawer.vue` 迁移到 `web/src/views/runtime/`。
- 将服务器专属 `ServerTerminal.vue` 迁移到 `web/src/views/cluster/`。
- 将工作负载专属 `PodTerminal.vue` 迁移到 `web/src/views/resources/`。
- 移动对应测试文件，并更新页面中的相对导入。
- 保留跨页面复用的 `SectionTabsHeader.vue` 在 `web/src/components/`。
- 保持路由、API 契约、组件行为、测试语义和构建产物不变。

## Capabilities

### New Capabilities

- `frontend-component-boundaries`: 按页面领域组织页面专属功能组件，并保留跨领域通用组件。

### Modified Capabilities

无。此变更只调整源码组织和依赖路径，不改变产品行为或接口要求。

## Impact

- 影响 `web/src/components/`、`web/src/views/monitoring/`、`web/src/views/runtime/`、`web/src/views/cluster/` 和 `web/src/views/resources/`。
- 影响 `Monitoring.vue`、`RuntimeManagement.vue`、`Servers.vue`、`Workloads.vue` 的组件导入路径。
- 影响相关组件测试文件的相对导入路径。
- 不影响后端、REST API、路由 URL、权限和运行时数据。
