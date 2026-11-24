## Why

系统设置的“发布与更新”页会在挂载、切换标签和定时轮询时刷新平台状态。较新的刷新会按既有生命周期约定取消旧请求，但发布页把取消后返回的空结果显示为“读取平台发布状态失败”，造成错误提示并干扰操作员判断。

## What Changes

- 将发布状态读取的取消结果视为非错误：已取消或已过期的读取不显示页面错误。
- 保留最新请求优先、请求取消、15 秒轮询和真实服务端失败的现有行为。
- 新增覆盖重叠刷新和真实失败的前端回归测试。
- **Non-goals:** 不改变 `/api/platform/status` 的 REST 合同、后端状态计算、轮询频率或发布操作的错误处理。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `frontend-platform-control-api-boundary`: 发布设置的受管读取在被取消时保持静默，仅渲染当前请求的真实失败。

## Impact

- 前端：`SystemSettingsRelease.vue` 及其测试。
- API、后端、数据模型、轮询周期和外部依赖不受影响。
