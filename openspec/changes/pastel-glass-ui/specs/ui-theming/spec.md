## MODIFIED Requirements

### Requirement: 配色选择完整性
系统 SHALL 提供 Reference Glass、Mist Blue、Orchid Glass、Sky Veil 和 Night Glass 五种可选配色，并将选择应用到概览、服务器、路由、证书、资源、审计、数据管理和登录页面的所有组件。

#### Scenario: 配色选择持久化
- **WHEN** 用户选择任一预设配色（包含天空紫雾）并刷新页面
- **THEN** 系统从 `localStorage` 读取 `cylism-palette` 并保持该配色

#### Scenario: 首次访问选择默认配色
- **WHEN** 用户首次访问且不存在已保存的配色
- **THEN** 系统在浅色系统偏好下使用 Reference Glass，在深色系统偏好下使用 Night Glass

#### Scenario: 配色切换即时生效
- **WHEN** 用户从配色菜单选择另一套预设
- **THEN** 所有可见组件立即更新颜色和玻璃表面，且不重新加载路由或数据

#### Scenario: 所有页面无主题残留
- **WHEN** 用户在任意一套预设配色下浏览所有已认证页面和登录页面
- **THEN** 文本、状态标签、表格、输入框、模态框和图标不存在与当前背景冲突的硬编码旧主题颜色
