## Why

前端已有可复用的 `SelectMenu`，但 25 个业务视图仍散落 73 个原生下拉框，导致同一类操作呈现出不同的视觉、焦点和禁用状态。将这些控件统一到共享组件，能让界面保持一致，同时避免后续页面继续复制原生控件实现。

## What Changes

- 扩展 `SelectMenu` 的表单、标识、测试定位和数值值兼容能力，使其能够无损承接现有原生下拉框的用法。
- 将所有业务视图中的原生 `<select>` 迁移为 `SelectMenu`，保留原有选项、默认值、占位提示、禁用条件和变更联动。
- 更新受影响的页面样式和前端测试，验证组件迁移不会改变 API 请求、提交值或页面业务状态。

## Capabilities

### New Capabilities

- `frontend-select-menu`: 定义共享下拉组件的交互、表单兼容性和值类型契约，以及业务视图统一采用该组件的要求。

### Modified Capabilities

- None.

## Impact

- 影响 `web/src/components/SelectMenu.vue` 及其单元测试。
- 影响使用原生下拉框的 25 个业务视图及相关前端测试。
- 不改变 REST 或 gRPC API、后端数据模型、选项业务含义或引入新的运行时依赖。
