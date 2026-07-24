## MODIFIED Requirements

### Requirement: Glass UI Design Tokens
项目 SHALL 拥有完整的语义化 Glass UI Design Tokens 体系，覆盖 Reference Glass、Mist Blue、Orchid Glass、Sky Veil 和 Night Glass 五套预设配色。每套配色 SHALL 定义画布、玻璃表面、文本、操作、焦点、状态和效果令牌，组件 SHALL 不直接使用原始调色板值。

#### Scenario: Reference Glass 令牌匹配图五
- **WHEN** 用户使用默认 Reference Glass 配色加载任意页面
- **THEN** 页面使用 `#eaf1f0` 画布、`#c2e4db` 上方色带、`#d8e4f7` 下方色带与 `#22736b` 操作色，且所有组件引用同一组语义令牌

#### Scenario: Night Glass 令牌完整
- **WHEN** 用户切换到 Night Glass 配色
- **THEN** 页面使用深石墨画布和深青玻璃层，同时文本、焦点、健康、警告和失败状态保持可读且语义一致

#### Scenario: 组件不使用原始颜色值
- **WHEN** 组件在任意预设配色下渲染
- **THEN** 组件颜色、背景、边框和阴影由语义 token 提供，不因硬编码颜色而残留旧 Soft Tech 外观

#### Scenario: 滚动条不占布局空间
- **WHEN** 页面内容需要纵向滚动
- **THEN** 滚动条以 overlay 模式出现，不导致内容区域宽度变化
