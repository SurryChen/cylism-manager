# ui-components Specification

## Purpose
TBD - created by archiving change ui-soft-tech. Update Purpose after archive.
## Requirements
### Requirement: 统一组件视觉
所有 UI 组件 SHALL 使用统一的圆角、阴影、间距和过渡动画，呈现现代化的 Soft Tech 风格。

#### Scenario: 卡片样式统一
GIVEN 任意页面的 .card 组件
WHEN 页面渲染
THEN 使用 8px 圆角、1px border、subtle 阴影、24px 内边距

#### Scenario: 按钮样式统一
GIVEN 任意 .btn 组件
WHEN 渲染
THEN 使用 6px 圆角、200ms hover 过渡、统一的 padding

#### Scenario: 模态弹窗统一样式
GIVEN 弹出 .modal
WHEN 渲染
THEN 使用 12px 圆角、8px 32px shadow、32px 内边距

#### Scenario: 表格行 hover 效果
GIVEN 数据表格渲染
WHEN 鼠标悬停某行
THEN 背景色过渡到 --bg-hover，动画曲线 200ms ease

