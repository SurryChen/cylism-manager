## Why

证书、监控和平台托管制品库页面仍在页面组件中直接调用通用 `api` 并拼接 REST 路径。监控筛选、证书安装状态和制品库证书/PVC 读取都可能与后续刷新重叠，页面级单一错误状态也会让局部失败覆盖其他区域的成功数据。

## What Changes

- 完善 `web/src/api/certificates.js`、`web/src/api/monitoring.js` 和相关制品库 API 模块，集中页面所需的读取与变更接口。
- 将 `Certificates.vue`、`Monitoring.vue` 和 `ManagedOCIRegistries.vue` 的读取请求接入现有 `useAsyncResource`，并为筛选、刷新、安装和迁移状态保留取消及过期响应保护。
- 将状态、趋势、告警、证书选项和制品库操作错误拆分为局部状态，刷新失败保留最近一次成功数据。
- 为 API 参数、请求竞态、轮询生命周期、卸载清理和关键变更失败补充测试。

## Capabilities

### New Capabilities

- `frontend-observability-api-boundary`: 证书、监控和托管制品库页面的 API 归属与请求生命周期。

### Modified Capabilities

- 无。

## Non-goals

- 不修改后端 REST 路由、认证逻辑、响应格式或 Kubernetes 资源行为。
- 不引入全局状态管理、缓存框架或新的 npm 依赖。
- 不迁移域名、服务器、存储和其他页面的 API 调用。
- 不在本次变更中拆分大块展示组件或重做页面视觉设计。

## Impact

- 受影响前端代码：`web/src/api/`、证书/监控/托管制品库页面及其测试。
- 保持现有 HTTP 方法、路径、payload、轮询间隔和用户可见操作不变。
