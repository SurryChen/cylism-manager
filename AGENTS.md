# AGENTS.md — Cylism Manager 项目约定

## 技术栈

- 后端: Go 1.22, Gin, GORM, SQLite
- 前端: Vue 3 + Vite
- 通信: gRPC (Agent), REST (Web UI)
- 认证: JWT (HMAC-SHA256), bcrypt

## 代码风格

- Go: 遵循标准 Go 风格，`gofmt` 格式化
- Vue: 使用 Composition API (`<script setup>`)
- 测试文件与源文件同目录，`*_test.go`
- 中文 UI 文案，英文代码标识符
- 使用 Design Tokens（CSS 变量）管理样式

## 前端页面与组件规范

以 `web/src/views/OperationHistory.vue` 的列表页布局为参考，优先复用 `web/src/components/` 中的组件和 `web/src/styles/` 中的通用样式，不为单个页面重复实现控件样式。

- 页面使用 `SectionTabsHeader` 承载标题、Tab 和右侧摘要或操作；即使只有一个视图也保留 Tab。标题下方直接进入内容区域，不叠加说明性卡片。
- 列表页优先采用一个 `SurfaceCard`，内部依次放筛选工具栏、`data-table` 表格和分页；不要在卡片内再嵌套卡片。卡片本身使用 `padding="none"`，由工具栏、表格和分页分别控制内边距。
- 桌面端筛选项在工具栏中横向排列，控件间距约 `8px`；新增等页面主操作放工具栏右侧，筛选按钮紧随筛选项。窄屏时筛选项改为单列并撑满宽度，表格保留横向滚动。
- 工具栏筛选控件默认采用紧凑单行布局；字段名称使用 `sr-only` 或 `aria-label` 提供无障碍标识，不在控件上方额外占用一行。多个筛选控件、摘要和主操作在桌面端保持同一基线对齐。
- 下拉框统一使用 `SelectMenu`，不要使用未封装的原生下拉框。文本输入使用 `form-input`；普通命令按钮使用 `btn`，主操作叠加 `btn-primary`，次要小按钮叠加 `btn-sm`。重置等含义明确的工具操作使用 `icon-button` 和 Lucide 图标，并提供 `title`、`aria-label`。
- 输入框、下拉框、普通按钮以约 `36px` 高度对齐；图标按钮约 `34 × 34px`，图标约 `15px`；`btn-sm` 约 `30px` 高。优先使用现有 Design Tokens，避免在页面中创造另一套尺寸。
- 表格统一使用 `data-table`：约 `10px` 表头、`12px` 单元格文字和细行分隔线；状态使用通用 `badge`，次要信息使用 `cell-secondary`。表格外侧桌面端约 `20px`、窄屏约 `14px` 内边距。
- 加载和无数据使用 `EmptyState`；页面内可恢复的读取错误使用 `FeedbackBanner`，需要集中提示或编辑、确认的场景使用通用弹窗组件。根据场景选择反馈形式，不为同类状态重复设计。
