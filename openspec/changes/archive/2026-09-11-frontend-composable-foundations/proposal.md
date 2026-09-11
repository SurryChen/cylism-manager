## Why

`web/src/composables` 目前只有异步资源、轮询和主题逻辑，但多个页面仍重复实现 Tab 查询参数同步、页面滚动锁定和异步操作状态管理。这些重复代码容易产生行为不一致和清理遗漏，适合先提炼为小而明确的 Vue composable。

## What Changes

- 新增 `useRoutedTab`，统一带 `tab` 查询参数的有效值校验、默认值和路由切换。
- 新增 `useBodyScrollLock`，统一弹窗、抽屉和终端场景的页面滚动锁定及生命周期清理。
- 新增 `useActionState`，统一异步写操作的运行状态、错误状态、成功返回值和异常清理。
- 将 Monitoring、Cluster、Network、Resources、Settings 等页面的重复 Tab 编排迁移到 `useRoutedTab`。
- 将服务器终端和 Pod 终端的滚动锁定迁移到 `useBodyScrollLock`。
- 将有限范围内的重复提交状态迁移到 `useActionState`，保持现有错误文案、请求参数、路由和页面行为不变。
- 为新增 composable 及关键迁移页面补充同目录测试。
- 不改变后端 API、路由 URL、业务规则、第三方依赖或用户可见功能。

## Capabilities

### New Capabilities

- `frontend-composable-foundations`: 提供可复用且生命周期安全的前端路由 Tab、页面滚动锁定和异步操作状态逻辑。

### Modified Capabilities

无。此次变更只抽取已有行为，不改变既有产品需求或接口契约。

## Impact

- 影响 `web/src/composables/` 及其测试文件。
- 影响使用 Tab 导航的领域页面、服务器/Pod 终端组件，以及少量异步操作页面。
- 影响页面内部的导入路径和状态编排，但不影响 REST API、路由 URL、认证、后端或部署资源。
