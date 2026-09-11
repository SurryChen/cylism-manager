## Why

`web/src/views` 目前所有路由页面都平铺在同一目录，应用、集群、Kubernetes 资源、网络和系统设置等不同领域混在一起。随着页面数量增加，定位页面、维护同目录测试和发现领域边界的成本持续上升；现在进行有限的领域分组可以改善可维护性，同时不改变用户可见的路由和功能。

## What Changes

- 按应用、集群、资源、网络和系统设置领域整理相关 Vue 页面及其同目录测试文件。
- 更新路由动态导入路径，以及移动页面后的相对依赖路径。
- 将系统设置共享样式与系统设置页面放在同一领域目录。
- 保持 URL、路由名称、组件行为、API 契约和测试语义不变。
- 不新增全局状态、第三方依赖或额外的组件层级。

## Capabilities

### New Capabilities

- `frontend-view-domain-organization`: 以有限领域分组组织前端路由页面和测试文件。

### Modified Capabilities

无。此变更只调整源码组织，不改变产品行为或接口要求。

## Impact

- 影响 `web/src/views/` 下页面、测试和共享样式文件。
- 影响 `web/src/router/index.js` 中的动态导入路径。
- 不影响后端、REST API、路由 URL、构建产物路径或运行时依赖。
