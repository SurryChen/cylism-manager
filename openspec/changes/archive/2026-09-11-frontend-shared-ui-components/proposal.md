## Why

`web/src/components/` 目前只保留了 `SectionTabsHeader.vue`，但多个页面仍然重复实现页面标题、区块标题、提示条、空状态和弹窗外壳。重复标记和样式会让可访问性、响应式行为及 Glass UI 细节逐渐出现偏差，也让页面文件承担过多结构性代码。

本变更提炼一组小而稳定的跨页面基础组件，收口公共结构与交互，同时明确业务状态、数据请求和领域工作区仍由各自的 view 持有。

## What Changes

- 新增通用 `PageHeader`，统一页面标题、描述、返回入口和操作区。
- 新增通用 `SectionHeading`，统一卡片或内容区标题、辅助说明和 actions 插槽。
- 新增通用 `FeedbackBanner`，统一成功、警告、错误和信息提示的语义及样式。
- 新增通用 `EmptyState`，统一加载中、无数据和引导操作的空状态呈现。
- 新增 `BaseModal` 与 `ConfirmDialog`，统一 overlay、关闭行为、焦点语义和确认操作区。
- 在少量代表性页面中迁移上述组件，验证 API、样式和测试约定后再继续扩展。
- 保留 `SectionTabsHeader.vue` 作为现有跨领域导航组件；不迁移监控、终端、表格等领域专属组件。
- 不改变页面路由、API 请求、业务状态、权限规则或用户可见文案含义。

## Capabilities

### New Capabilities

- `frontend-shared-ui-components`: 为多个页面提供可访问、主题一致且不携带业务状态的通用 UI 基础组件。

### Modified Capabilities

无。此次变更主要抽取已有页面结构，不改变现有产品能力或接口契约。

## Impact

- 影响 `web/src/components/`，新增基础组件及其同目录测试。
- 影响少量代表性 `web/src/views/` 页面及其测试中的模板和导入。
- 可能调整 `web/src/styles/components.css` 中与组件重复的基础样式，但不引入新的样式系统或第三方依赖。
- 不影响后端、REST API、路由 URL、构建产物和部署资源。
