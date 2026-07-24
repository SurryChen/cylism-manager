## MODIFIED Requirements

### Requirement: 统一 Glass UI 组件视觉
所有 UI 组件 SHALL 使用统一的 Glass UI 圆角、半透明表面、边框、阴影、间距和交互过渡。玻璃效果 SHALL 用于结构性面板，不得将每个数据行嵌套为独立卡片。

#### Scenario: 面板样式统一
- **WHEN** 任意页面渲染数据面板、指标面板、筛选工具栏或空状态容器
- **THEN** 它使用语义玻璃表面、约 14px 圆角、1px 玻璃边框和统一阴影，并在不支持模糊时降级为不透明表面

#### Scenario: 按钮与图标样式统一
- **WHEN** 任意 `.btn` 或图标按钮渲染
- **THEN** 它使用统一 padding、焦点环、hover 过渡和语义操作颜色，且图标按钮具有可访问名称或 tooltip

#### Scenario: 模态弹窗统一样式
- **WHEN** 弹出 `.modal`
- **THEN** 它使用 raised glass 表面、统一圆角、可读 overlay 和固定的标题/操作区域，不遮挡键盘焦点

#### Scenario: 表格行 hover 效果
- **WHEN** 数据表格渲染且用户悬停一行
- **THEN** 该行背景过渡到语义 `table-row-hover` 表面，保留列分隔、状态可读性和 200ms 以内的过渡

#### Scenario: 减少动态效果
- **WHEN** 用户的系统启用 `prefers-reduced-motion`
- **THEN** 组件禁用非必要的玻璃和导航过渡，不影响内容展示或交互
