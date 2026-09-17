## Why

`SurfaceCard` 已经成为部分页面的共享玻璃面板入口，但大量内容面板仍通过全局 `.card`、`.card-header` 和 `.card-title` 约定构建。两套并行外壳让视觉调整、可访问性约束和移动端间距无法集中验证；节点镜像源页面也缺少与制品库一致的结构面板。

现在迁移可在不改变任何业务工作流的前提下，收敛所有内容面板的结构入口，并移除不再需要的 `.card` 外壳样式。

## What Changes

- 将节点镜像源的规则列表工作区接入 `SurfaceCard`，保留紧凑表格、筛选以外的现有操作和所有弹窗流程。
- 按领域将遗留内容面板的 `.card` 容器迁移为 `SurfaceCard`，并使用现有的语义容器、内边距和头部/操作插槽。
- 保留页面拥有的标题、说明、表格、列表、空状态和业务按钮；不为单条记录额外嵌套卡片。
- 在所有内容面板完成迁移后，移除全局 `.card` 的玻璃外壳和响应式内边距规则；保留与 `.card` 无关的 `.metric`、`.modal`、`.k8s-banner` 规则。
- 为迁移后的页面补充或更新组件使用断言，验证页面行为与可见结构保持不变。

## Capabilities

### New Capabilities

- `legacy-card-surface-adoption`: 将遗留内容面板统一到既有 `SurfaceCard` 外壳，同时保持运维页面的行为、信息密度和响应式布局。

### Modified Capabilities

- None.

## Impact

- 修改 `web/src/views/` 下使用 `.card` 作为内容面板的 Vue 视图及其现有 Vitest 测试。
- 修改 `web/src/styles/components.css`，删除 `.card` 外壳规则并保留非卡片表面样式。
- 依赖已完成但尚待归档的 `add-glass-surface-card` 变更提供的 `SurfaceCard`；不修改后端 API、路由、数据模型或引入依赖。
