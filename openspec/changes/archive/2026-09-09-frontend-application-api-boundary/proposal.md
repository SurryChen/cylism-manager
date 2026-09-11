## Why

`Applications.vue` 和 `ApplicationDetails.vue` 已经使用部分领域 API 模块，但创建、更新、删除、发布、重启和控制台跳转等操作仍直接调用通用 `api` 并在页面中拼接 REST 路径。详情切换和关联资源刷新也存在多个页面级请求入口，后续维护容易出现请求覆盖、错误提示互相覆盖和接口契约漂移。

## What Changes

- 完善 `web/src/api/applications.js`，集中应用工作台、项目、发布、模板、端点和工作负载切换相关的读取与变更接口。
- 完善或新增 `web/src/api/application-details.js`（如边界清晰且复用价值足够），集中应用详情、能力标签、集成委托和模板/端点操作；避免为了单个函数创建文件。
- 将两个页面的读取请求统一接入现有 `useAsyncResource` 生命周期，保证路由参数变化时取消旧请求、忽略过期结果，并在刷新失败时保留上次成功数据。
- 将列表、详情、轮询和变更错误拆分为局部状态，保持现有中文提示和用户流程。
- 为 API 参数、请求竞态、卸载清理、关键变更失败和页面回归流程补充测试。

## Capabilities

### New Capabilities

- `frontend-application-api-boundary`: 应用工作台与应用详情的 API 归属和请求生命周期。

### Modified Capabilities

- 无。

## Non-goals

- 不修改后端 REST 路由、认证逻辑、请求 payload 或返回数据结构。
- 不引入全局状态管理、缓存框架或新的 npm 依赖。
- 不在本次变更中迁移证书、监控、服务器等其他页面。
- 不为了降低行数拆分应用页面的纯展示模板。

## Impact

- 受影响前端代码：`web/src/api/`、`Applications.vue`、`ApplicationDetails.vue` 及对应测试。
- 只调整前端调用边界和错误/请求状态，不改变用户可见业务能力。
