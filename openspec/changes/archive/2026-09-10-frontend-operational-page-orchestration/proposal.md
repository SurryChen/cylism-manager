## Why

`Servers.vue` 和 `Workloads.vue` 是平台用户最常用的运维页面，但目前一个页面同时管理清单、详情、轮询、变更操作和多个弹窗，部分请求与错误状态仍会互相影响。现在 API 领域边界已经收口，适合进一步稳定页面编排，降低快速切换、操作失败和页面离开时出现错误状态错位的风险。

## What Changes

- 稳定 `Servers.vue` 的服务器清单、资源监控、网络诊断、单服务器统计和变更操作状态。
- 稳定 `Workloads.vue` 的资源清单、工作负载详情、扩缩容、镜像更新和回滚状态。
- 让刷新失败保留最近一次成功数据，并将错误显示在对应的页面区域。
- 让变更失败保留相关弹窗和用户输入，释放提交状态并支持重试。
- 防止旧的详情或统计请求覆盖用户当前选中的工作负载或服务器。
- 确保页面卸载时取消可取消请求并停止轮询，不更新已销毁的页面状态。
- 为上述行为补充与领域 API 模块边界一致的页面回归测试。

## Capabilities

### New Capabilities

- `frontend-operational-page-orchestration`: 高频服务器和工作负载页面的请求编排、局部状态隔离、失败恢复与生命周期安全。

### Modified Capabilities

无。

## Impact

- 影响 `web/src/views/Servers.vue`、`web/src/views/Workloads.vue` 及其同目录测试。
- 可能新增小型页面级 composable，但不引入全局状态管理或新的 npm 依赖。
- 复用现有 `web/src/api/servers.js`、`web/src/api/kubernetes.js`、`useAsyncResource` 和 `usePolling`。
- 不修改后端接口、路由、Kubernetes 资源语义、认证逻辑或 API 请求协议。
