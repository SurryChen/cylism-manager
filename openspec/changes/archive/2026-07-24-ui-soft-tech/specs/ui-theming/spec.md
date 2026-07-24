## ADDED Requirements

### Requirement: 主题切换完整性
主题切换 SHALL 覆盖所有页面组件，亮色模式下无硬编码暗色值残留。

#### Scenario: 主题切换持久化
GIVEN 用户切换到亮色模式
WHEN 刷新页面
THEN 保持亮色模式（从 localStorage 读取）

#### Scenario: 亮色模式无暗色残留
GIVEN 用户在亮色模式
WHEN 浏览所有页面（概览、服务器、路由、证书、资源、审计、数据管理、登录）
THEN 无硬编码的 #fff 文字在白色背景上不可见的问题

#### Scenario: 主题切换即时生效
GIVEN 用户点击主题切换按钮
WHEN 主题变化
THEN 页面颜色即时切换，无闪烁或等待
