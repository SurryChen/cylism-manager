## ADDED Requirements

### Requirement: Soft Tech Design Tokens
项目 SHALL 拥有完整的 Design Tokens 体系，覆盖亮/暗两套主题，使用暖灰底色和紫蓝强调色。

#### Scenario: 暗色主题变量完整
GIVEN 用户在暗色模式下
WHEN 加载任意页面
THEN bg-deep 为 #0f0f14，accent 为 #7c6ff7，所有组件颜色一致

#### Scenario: 亮色主题变量完整
GIVEN 用户切换到亮色模式
WHEN 加载任意页面
THEN bg-deep 为 #f5f5fa，accent 为 #6c5ce7，无硬编码暗色残留

#### Scenario: 滚动条不占布局空间
GIVEN 页面内容需要纵向滚动
WHEN 页面渲染
THEN 滚动条以 overlay 模式出现，不导致内容区域宽度变化
