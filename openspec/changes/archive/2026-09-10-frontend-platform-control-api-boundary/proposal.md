## Why

系统组件、平台入口/发布/临时访问令牌和集群 DNS 都是高权限运维操作，但其 mutation 仍散落在页面中直接拼接 REST 路径。现有读取已经部分迁移到领域 API 模块，补齐写操作和读取生命周期可以让请求契约、取消行为与失败反馈保持一致，并降低后续修改平台控制面的回归风险。

## What Changes

- 扩展 `system-components.js`，使系统组件配置保存和恢复默认配置由领域 API 模块拥有。
- 扩展 `settings.js`，覆盖平台入口、发布、Webhook secret 和临时访问令牌的读写操作。
- 新增 `cluster-dns.js`，拥有 DNS 策略读取、保存、清除和历史版本回滚操作。
- 将 `SystemComponents.vue`、系统设置子页面和 `ClusterDNS.vue` 迁移到命名领域 API 函数，并为可重叠读取使用现有 `useAsyncResource`。
- 补充 API 合同、取消/旧响应隔离及 mutation 失败后保留操作界面的测试。

## Capabilities

### New Capabilities

- `frontend-platform-control-api-boundary`: 平台控制面页面的领域 API 所有权、可取消读取和局部错误反馈。

### Modified Capabilities

- 无。

## Impact

- 影响 `web/src/api/system-components.js`、`web/src/api/settings.js`，并新增 `web/src/api/cluster-dns.js` 及对应测试。
- 影响 `SystemComponents.vue`、`SystemSettingsEntry.vue`、`SystemSettingsRelease.vue`、`SystemSettingsSecurity.vue`、`ClusterDNS.vue` 及其测试。
- 不修改后端 endpoint、HTTP 方法、请求 payload、认证方式、发布流程或 DNS 策略语义。
