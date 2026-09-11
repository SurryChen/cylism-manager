## Context

页面中已经形成稳定的重复 UI 结构：

- `page-header` 在多个资源、应用和运维页面中重复出现；
- `card-header`、`k8s-banner` 和 `empty-state` 在列表、状态和错误场景中重复出现；
- `overlay`、`modal`、`modal-actions` 在创建、编辑、删除和详情交互中重复出现。

这些结构本身不属于任何一个业务领域，但目前由各个 view 以不同方式实现。组件抽取应改善一致性，同时避免把领域工作区或业务编排重新放回全局组件目录。

## Goals / Non-Goals

**Goals:**

- 提供少量、可组合且无业务依赖的公共 UI 组件。
- 统一语义 HTML、ARIA 属性、键盘关闭和移动端布局。
- 复用现有 Design Tokens 与 Glass UI 样式，不引入第三方组件库。
- 保持页面继续负责 API 请求、业务状态、表单校验和领域专属样式。
- 为每个新增组件提供同目录测试，并用代表性页面验证迁移。

**Non-Goals:**

- 不把监控、日志、终端、工作负载表格或告警工作区抽成全局组件。
- 不把所有现有页面一次性迁移，也不为了抽取组件而重写页面业务逻辑。
- 不新增 Pinia、全局状态、路径别名或第三方依赖。
- 不改变现有 API、路由 URL、权限和用户可见业务文案。
- 本次不纳入 `RefreshButton`、`StatusBadge`、`QuantityInput` 等候选组件；待第一批稳定后再单独评估。

## Decisions

### 1. 组件只封装结构和交互，不拥有业务状态

组件通过 props、slots 和事件接收标题、提示内容、按钮和可见状态。它们不得直接发起 API 请求、读取路由、访问全局 store 或决定业务错误文案。

建议的最小接口如下：

| Component | 主要输入 | 插槽/事件 |
| --- | --- | --- |
| `PageHeader` | `title`、可选 `description`、可选 `backTo` | `actions` 插槽；返回事件或普通链接行为 |
| `SectionHeading` | `title`、可选 `description` | `actions` 插槽 |
| `FeedbackBanner` | `tone`、`message`、可选 `dismissible` | `default` 插槽；`dismiss` 事件 |
| `EmptyState` | `message`、可选 `variant`、可选 `icon` | `action` 插槽 |
| `BaseModal` | `open`、可选 `title`、可选 `size` | `default`、`actions` 插槽；`close` 事件 |
| `ConfirmDialog` | `open`、`title`、`message`、确认/取消文案、`busy` | `confirm`、`cancel`、`close` 事件 |

具体 prop 名称可在实现阶段根据现有模板和测试约定微调，但必须保持职责边界：页面持有状态，组件只负责呈现和交互通知。

### 2. 保留页面自定义 class 和插槽

组件提供稳定的根节点、`class` 透传或 `variant`/`size` 选项，使代表性页面能够保留必要的布局 class（例如详情弹窗宽度、页面特定 z-index），而不复制组件实现。

### 3. 使用语义与可访问性作为统一契约

- `PageHeader` 使用 `header` 和单一 `h1`；
- `FeedbackBanner` 默认使用 `role="alert"`，信息型提示可显式关闭该语义；
- `BaseModal` 使用 `role="dialog"`、`aria-modal="true"`，有标题时关联 `aria-labelledby`；
- 弹窗支持 Escape 关闭、点击 overlay 是否关闭由显式 prop 控制，并避免在关闭时遗留 body 滚动锁；
- 确认操作在 `busy` 时禁用，避免重复提交。

组件不自行实现复杂焦点陷阱；如果现有页面已有焦点或滚动生命周期逻辑，迁移时必须保持原页面行为。

### 4. 先迁移少量代表性页面

首批优先覆盖结构差异明显但业务风险可控的页面：

- `AuditLogs.vue`：页面标题、错误提示、空状态和详情弹窗；
- `Applications.vue`：页面标题、提示条、空状态和创建/删除弹窗；
- `ClusterDNS.vue` 或 `Configs.vue`：提示条、空状态和确认弹窗；
- `Monitoring.vue`：状态提示、等待状态和区块标题。

不要求本次清理所有重复 class。迁移后保留仍被未迁移页面使用的全局 CSS，待引用归零且回归验证完成后再清理。

## Risks / Trade-offs

- [抽象接口过度通用] → 只实现第一批六个基础组件，所有业务字段和操作仍通过 slots/props 注入。
- [迁移引入视觉回归] → 复用现有 Design Tokens，保留页面特定 class，并执行组件测试、代表性 view 测试和 Vite 构建。
- [弹窗关闭行为改变] → 将 Escape、overlay click、busy 和 unmount 行为写入组件测试，迁移时逐页对照原逻辑。
- [全局样式提前删除导致旧页面破坏] → 只有在全仓库搜索确认无引用后才删除重复样式；本 change 默认不做激进清理。
- [组件目录重新混入领域逻辑] → 评审新增组件是否依赖具体 API、路由或领域模型；不符合条件的组件留在对应 view 目录。

## Migration Plan

1. 为六个组件定义 props、slots、事件和可访问性测试。
2. 在 `web/src/components/` 实现组件，并复用现有 CSS 变量。
3. 迁移首批代表性页面和同目录测试，保持 API 请求与业务状态不变。
4. 使用 `rg` 检查组件使用点、残留重复结构和未使用导入；确认领域专属组件仍位于对应 view。
5. 运行前端测试、Vite 生产构建和 OpenSpec 校验。

回滚方式：恢复代表性页面的模板导入并删除新增组件，不涉及数据库、部署资源或外部系统迁移。

## Open Questions

暂无。`RefreshButton`、`StatusBadge` 和 `QuantityInput` 是否纳入下一轮，等第一批组件完成真实页面迁移后再根据复用数量决定。
