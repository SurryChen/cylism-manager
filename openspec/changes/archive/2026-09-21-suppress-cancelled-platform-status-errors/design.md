## Context

`SystemSettingsRelease.vue` 使用 `useAsyncResource` 读取平台状态。该 composable 在新读取启动时取消前一读取，并让取消或过期读取以 `undefined` 完成，同时不记录资源错误。发布页目前将所有假值结果都映射为用户可见的读取失败，因此错误地把正常取消显示为故障。

## Goals / Non-Goals

**Goals:**

- 取消或过期的发布状态读取不显示错误。
- 当前请求的真实失败仍显示原有错误提示。
- 最新成功结果继续更新发布信息和镜像前缀表单。

**Non-Goals:**

- 不修改 `useAsyncResource` 的通用返回契约。
- 不合并、去除或调整系统设置的轮询和标签刷新触发点。
- 不隐藏真实 HTTP、鉴权或响应解析失败。

## Decisions

### 在发布页区分取消和资源错误

发布页调用 `platformResource.refresh()` 后，仅当资源记录了真实错误时才写入 `platformReadError`。资源没有错误且未返回当前结果，表示该读取已被取消或被较新读取取代，页面保持静默并由较新的刷新负责更新状态。

备选方案是修改 `useAsyncResource` 返回一个带状态的结果对象。该 composable 有大量既有调用方；为了修复单一页面的呈现错误而改变公共契约会扩大回归面，故不采用。

## Risks / Trade-offs

- [接口未来合法返回空值] -> 当前 `/platform/status` 合同返回对象；以资源错误状态作为用户错误的唯一依据，并用测试固定该行为。
- [隐藏真实失败] -> 覆盖拒绝请求场景，确保真实错误仍显示服务端信息。
- [重叠请求时旧结果写入表单] -> 保留 `useAsyncResource` 的 request ID 与 abort 语义，只有当前结果可被页面提交。

## Migration Plan

1. 先添加发布页重叠刷新和真实失败的测试。
2. 以最小改动修正发布页的错误分支。
3. 运行聚焦测试、全部前端测试和生产构建。
4. 回滚只需恢复发布页的本地错误分支；无需数据迁移。

## Open Questions

- 无。
