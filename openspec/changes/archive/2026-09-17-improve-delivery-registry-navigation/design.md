## Context

当前交付中心只有 `/delivery/registry` 路由。`ManagedOCIRegistries.vue` 只在创建时手工解析 `window.location.hash` 判断 Proxy Tab，随后 Tab 点击只更新组件内部 ref。因此 URL 并不代表当前工作区，也没有使用项目统一的 Tab 路由工具。Registry Proxy 卡片又在局部样式中使用 `--surface-subtle` 和弱边框，未使用同页自托管 Registry 指标卡所共享的玻璃表面。

自托管制品库目前还将低频的服务运维（地址、健康、存储、配置和删除）与高频的制品浏览（仓库、Tag、复制拉取地址）连续堆叠。没有选中仓库时，Tag 面板是大片空白，且工具栏没有明确的工作流边界。Proxy 诊断接口无论成功或失败都会将可读摘要写到 `last_diagnostic_error`，而界面按该字段是否存在直接使用错误色，导致成功的“代理 Pod 可访问上游 Registry”也显示为红字。

## Goals / Non-Goals

**Goals:**

- 为两个顶层制品库工作区提供稳定、可复制、可刷新且支持浏览器历史的 query-tab URL。
- 与项目其他聚合页一致地使用 `?tab=` 表示当前工作区。
- 让 Proxy 实例卡复用自托管 Registry 指标卡的玻璃表面，同时保留密集的属性网格与状态信息。
- 让服务维护与镜像目录各有清晰的入口、动作归属和非空默认状态。
- 让 Proxy 健康、未诊断和失败状态由诊断状态码决定，而非仅依据摘要字段是否为空。
- 以组件和路由测试覆盖直接访问、Tab 切换、历史 URL 迁移和卡面契约。

**Non-Goals:**

- 不改变 Registry Proxy REST API、表单、诊断、DNS、缓存清理或迁移语义。
- 不改变自托管制品库的 catalog API、删除保护或表单配置流程。
- 不在本次将自托管制品库拆分为“概览 / 镜像”等额外工作区，也不新增侧栏入口。
- 不调整全局色带、调色板或其他页面的卡片样式。

## Decisions

### 1. 将 Proxy 作为制品库聚合页中的查询 Tab

保留 `/delivery/registry` 作为聚合页路径，通过 `tab` 查询参数区分工作区。自托管 Registry 使用 `?tab=registry`，Registry Proxy 使用 `?tab=registry-proxy`；无 `tab` 参数时仍默认显示自托管 Registry，以兼容既有侧栏入口。

已有 `/delivery/registry?tab=registry-proxy` 链接保持有效，无需迁移或重定向。

备选是将 Proxy 拆为 `/delivery/registry/proxy` 子路径。虽然可独立直达，但与集群、资源、网络、监控等聚合页的 `?tab=` 约定不一致，因此不采用。

### 2. 由 Vue Router 派生激活状态

`ManagedOCIRegistries` 使用现有 `useRoutedTab` 根据 `route.query.tab` 派生 Tab，并通过其 `router.push` 逻辑切换查询参数；不再读取 `window.location.hash` 或维护可与路由不同步的本地 active ref。`SectionTabsHeader` 保持通用选择组件，其选择事件由视图层转换为路由导航。

这使浏览器的刷新、前进、后退和直接访问天然保持正确状态。

### 3. Proxy 实例卡复用指标卡玻璃表面

每个 Proxy 实例使用共享的 `metric` 卡面类，再以局部 `proxy-instance` 规则覆盖其信息密度和零内边距需求。共享类提供 `--surface-glass`、`--border`、`--shadow-soft`、`--radius-panel` 和 backdrop blur；局部属性网格继续使用 `--border-muted` 作为内部列/行分隔。

备选是在 Proxy 局部重复玻璃 token。这样短期视觉相同，但会再次产生两套风格来源，未来难以保持同步，因此不采用。

### 4. 按任务频率分隔自托管 Registry 工作区

已部署 Registry 的主体分为两个未嵌套 Tab 的页面区段：`运行概览` 展示地址、健康、存储和服务级操作；`镜像目录` 展示仓库、标签和镜像操作。刷新、修复、配置、删除归入运行概览标题旁，镜像目录标题、过滤和数量归入目录标题旁。删除现有搜索工具栏的上下边界线，避免其被误解为独立卡片。

目录首屏成功读取且没有选中仓库时，自动选中返回列表中的第一个仓库并加载其标签；删除当前仓库后同样选择剩余列表的首项。筛选不改变当前选择，以免用户输入搜索词时重新请求标签。

备选是新增“概览 / 镜像”嵌套 Tab。该方案会在已有顶层工作区 Tab 下制造第二层导航，增加回退和链接状态复杂度，因此不采用。

### 5. 用诊断状态决定 Proxy 连通性样式

移除 Proxy 页内重复标题。属性网格的最后一个单元使用 `上游连通性`：显示诊断状态标签和 `last_diagnostic_error` 中的安全摘要。`healthy` 使用成功色，尚未诊断使用中性色，其他诊断状态使用错误色；`last_error` 是部署或生命周期错误，才保留单独的错误段落。这样健康诊断摘要不会再被错误地标红。

## Risks / Trade-offs

- [无 `tab` 参数的既有书签] -> 保留默认自托管 Registry 行为，并用视图测试保护。
- [路径切换造成 Registry 资源与 catalog 请求重复] -> 视图保留现有资源加载语义；测试仅要求工作区切换不改变 API 契约。
- [共享 `metric` 类的默认内边距干扰 Proxy 信息网格] -> 局部样式显式保留 `padding: 0` 与现有内部网格尺寸，并测试卡片的共享类和内部结构。
- [自动选中仓库增加一次标签读取] -> 仅在目录首次成功加载、当前没有选中项时读取；页面加载即有可用标签内容，减少空白状态。
- [诊断摘要字段名称带有 `error`] -> 以 `last_diagnostic_status` 作为唯一色彩依据，并仅把摘要作为该状态的补充文字。

## Migration Plan

1. 增加失败的路由、受管 Registry 目录工作流与 Proxy 诊断呈现测试。
2. 接入 `useRoutedTab`，将 Tab 选择绑定到 Vue Router query 参数。
3. 重排受管 Registry 的概览和目录操作区，并自动选择首个仓库。
4. 将 Proxy 实例卡迁移到共享玻璃卡表面，并将上游连通性收敛到属性网格。
5. 运行前端测试与 Vite build；回滚时可恢复既有页面结构，数据和后端无需回滚。

## Open Questions

- None.
