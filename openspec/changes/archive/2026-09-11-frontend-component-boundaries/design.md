## Context

前端已经按应用、集群、资源、网络和设置整理了路由页面，但 `web/src/components/` 仍混合两种内容：

- `SectionTabsHeader.vue`：被多个领域页面复用的通用页面导航组件；
- 监控工作区、Agent 聊天抽屉、服务器终端和 Pod 终端：分别只服务一个页面或一个明确领域的业务功能组件。

继续把页面专属组件放在全局组件目录，会让组件目录承担业务模块容器的职责，也会增加后续定位和维护成本。

## Goals / Non-Goals

**Goals:**

- 让页面专属组件与所属 `views` 领域相邻。
- 让 `web/src/components/` 只保留真正跨页面复用的组件。
- 保持组件文件独立，不把大组件直接合并进页面。
- 保持组件测试与源文件同目录。
- 保持导入路径、运行时行为、路由和 API 契约不变。

**Non-Goals:**

- 不改动组件内部业务逻辑、模板、样式和状态机。
- 不抽取新的终端 composable 或全局状态。
- 不将 `SectionTabsHeader.vue` 复制到各个领域。
- 不迁移 `api/`、`composables/`、`utils/` 或后端代码。
- 不新增第三方依赖、路径别名或全局组件注册。

## Decisions

### 1. 按唯一使用领域迁移页面专属组件

迁移关系如下：

| 当前组件 | 新位置 | 归属依据 |
| --- | --- | --- |
| `AlertingWorkspace.vue` | `views/monitoring/` | 只由监控页面使用 |
| `LoggingWorkspace.vue` | `views/monitoring/` | 只由监控页面使用 |
| `DiskGrowthWorkspace.vue` | `views/monitoring/` | 只由监控页面使用 |
| `MetricTrendChart.vue` | `views/monitoring/` | 当前只展示监控趋势数据 |
| `ChatDrawer.vue` | `views/runtime/` | 只由 Runtime 管理页面使用 |
| `ServerTerminal.vue` | `views/cluster/` | 只由服务器页面使用 |
| `PodTerminal.vue` | `views/resources/` | 只由工作负载页面使用 |

备选方案：将这些组件直接合并到页面文件。该方案会进一步扩大 `Monitoring.vue`、`RuntimeManagement.vue`、`Servers.vue` 和 `Workloads.vue`，破坏独立测试和生命周期边界，因此不采用。

### 2. 保留 SectionTabsHeader 为全局通用组件

`SectionTabsHeader.vue` 被监控、服务器、集群、Runtime 和系统设置多个页面使用，且只承担标题、Tab 和 actions 插槽渲染，因此继续保留在 `web/src/components/`。

备选方案：复制到各领域目录。复制会造成实现漂移和修复遗漏，因此不采用。

### 3. 测试文件跟随组件移动

组件的 `*.test.js` 与源文件一起迁移，保持现有同目录测试规范。页面中的导入路径调整为从新目录解析 API、组合式逻辑和组件依赖。

备选方案：建立独立的 `web/tests/components/` 测试树。该方案会增加源码和测试之间的路径距离，不符合当前项目约定，因此不采用。

## Risks / Trade-offs

- [相对导入遗漏] → 对迁移后的 Vue 和测试文件执行全量旧路径搜索，并运行前端测试与构建。
- [组件被遗漏在全局目录] → 以全仓库 import 使用点为依据，迁移后确认 `components/` 只剩 `SectionTabsHeader.vue`。
- [领域归属未来变化] → 如果组件出现第二个领域使用方，再单独评估提升回 `components/`，不提前引入抽象。
- [测试路径变化导致 mock 失效] → 保持测试内容不变，只调整必要的相对 import 路径。

## Migration Plan

1. 创建 `views/monitoring/` 和 `views/runtime/` 目录。
2. 移动页面专属组件及其测试文件到对应领域目录。
3. 更新页面和迁移组件内部的相对导入。
4. 更新测试导入路径，并检查 `components/` 残留文件。
5. 运行前端测试、Vite 构建、OpenSpec 校验和仓库级验证。

回滚方式：恢复文件移动和相对路径修改即可，不涉及数据库、部署资源或外部系统。

## Open Questions

暂无。后续若 `MetricTrendChart` 被非监控页面复用，再单独评估是否提升为通用组件。
