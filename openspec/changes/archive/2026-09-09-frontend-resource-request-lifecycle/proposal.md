## Why

应用工作台、服务器和监控页面会因路由参数、标签、时间范围或区段切换发起新的读取请求。当前页面直接使用通用 `api` 写入本地状态，较早请求晚返回时可能覆盖最新页面状态，且加载、错误与取消逻辑在每个页面重复实现。

## What Changes

- 为应用、服务器、监控读取接口建立小型领域 API 模块，集中路径、查询参数与可选 `AbortSignal` 传递。
- 将三个页面中可因页面状态变化重新发起的读取请求接入 `useAsyncResource`，取消过期请求并仅接受最新结果。
- 保留现有页面的提交、弹窗、轮询和局部业务状态；读取失败在对应页面区域保留可见错误状态。
- 为竞态、取消、领域 API 参数和三页的关键读取流程补充单元测试。

## Capabilities

### New Capabilities

- `frontend-resource-request-lifecycle`: 管理高频前端页面读取请求的取消、最新结果采纳与错误状态。

### Modified Capabilities

- 无。

## Impact

- 受影响前端代码：`web/src/api/`、`web/src/composables/useAsyncResource.js`、`Applications.vue`、`Servers.vue`、`Monitoring.vue` 及其测试。
- 后端 REST 路由、认证模型、数据格式和部署配置均不变。
- 不增加 npm 依赖，不引入全局状态管理或缓存框架。
