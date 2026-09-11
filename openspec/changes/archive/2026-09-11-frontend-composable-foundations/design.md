## Context

前端页面已经使用 `useAsyncResource` 和 `usePolling` 处理读取请求与定时器，但仍有三类横跨多个页面的基础逻辑散落在各个 Vue 文件中：

- `Monitoring`、`ClusterHub`、`NetworkHub`、`ResourceHub` 和 `SystemSettings` 重复从 `route.query.tab` 推导当前 Tab，并手动调用 `router.push`。
- `ServerTerminal` 和 `PodTerminal` 直接修改 `document.body.style.overflow`，未来其他全屏终端或抽屉容易重复实现清理逻辑。
- 多个页面在保存、安装、删除或更新操作中重复维护 `running/error` 状态和 `try/catch/finally` 生命周期。

本 change 只抽取通用机制，不抽取具体领域业务状态，也不引入全局状态管理。

## Goals / Non-Goals

**Goals:**

- 提供三个职责单一、可独立测试的 composable。
- 保持现有页面的路由 URL、请求参数、错误文案和用户操作行为。
- 让 Tab、滚动锁定和异步操作具备一致的生命周期清理。
- 让 composable API 足够小，页面仍能清楚表达自己的业务流程。

**Non-Goals:**

- 不重构所有页面的弹窗、表单或业务状态。
- 不把领域专属逻辑提升为全局 composable。
- 不替换现有 `useAsyncResource` 或 `usePolling`。
- 不新增 Pinia、状态管理框架、UI 框架或其他 npm 依赖。
- 不修改后端接口、路由定义或 API 模块。

## Decisions

### 1. 使用 `useRoutedTab` 统一 Tab 导航

`useRoutedTab({ router, route, tabs, defaultTab, path })` 返回 `activeTab` 和 `selectTab`。`activeTab` 只接受 `tabs` 中存在的 ID，否则返回 `defaultTab`；`selectTab` 保留现有 query 参数并更新 `tab`。

这样可以统一当前五个 Hub/Settings 页面中的重复实现，同时保留页面自行决定 Tab 列表和默认值的能力。

备选方案：在路由配置中增加全局 Tab 解析守卫。该方案会把页面展示状态耦合到路由层，并增加无关页面的复杂度，因此不采用。

### 2. 使用引用计数的 `useBodyScrollLock`

`useBodyScrollLock(locked)` 接收布尔值或响应式值。锁定时保存当前 body overflow，解锁或作用域销毁时恢复；多个组件同时锁定时使用模块级引用计数，避免一个组件卸载时误解除另一个组件的锁定。

备选方案：继续在每个终端或弹窗组件中手动设置 `overflow`。该方案简单但容易遗漏恢复和嵌套场景，因此不采用。

### 3. 使用轻量 `useActionState`

`useActionState` 提供 `running`、`error`、`result`、`run(action)` 和 `reset()`。`run` 负责设置运行状态、清理上一次错误、捕获异常并在 `finally` 中恢复状态；错误对象原样保留，页面继续决定展示文案和成功后的刷新动作。

该 composable 只管理异步生命周期，不负责请求、提示弹窗、重试策略或页面数据刷新，避免与 `useAsyncResource` 重叠。

备选方案：将 mutation 也并入 `useAsyncResource`。读取资源和写操作的状态语义不同，强行合并会使现有 API 变复杂，因此不采用。

### 4. 先迁移有限页面，再评估扩展

首批迁移 Tab 页面、两个终端组件和少量结构相似的保存/安装操作。每次迁移保持原变量或通过最小适配保留模板语义，不进行大规模页面重写。

备选方案：一次性迁移所有 `try/catch/finally`。该方案改动面过大，难以区分抽象收益和回归问题，因此不采用。

## Risks / Trade-offs

- [Tab composable 意外丢失页面专属 query 参数] → `selectTab` 基于现有 `route.query` 合并，只覆盖 `tab`，并为各 Hub 页面保留路由测试。
- [多个 Teleport/终端同时锁定导致滚动状态错乱] → 使用引用计数和原始 overflow 快照，并补充嵌套锁定与作用域销毁测试。
- [异步操作错误被吞掉或错误状态残留] → `run` 返回成功结果、重新抛出错误供页面选择处理，并测试成功、失败、重复执行和 finally 清理。
- [过度抽象使页面更难读] → composable 不包含领域 API、业务文案和弹窗状态，只抽取机械性生命周期逻辑。

## Migration Plan

1. 先为三个 composable 编写单元测试，再实现最小 API。
2. 迁移五个 Tab 页面，运行对应页面测试。
3. 迁移 `ServerTerminal`、`PodTerminal` 的滚动锁定，运行终端测试。
4. 迁移有限的异步 mutation 页面，逐页验证错误和表单保留行为。
5. 运行前端全量测试和 Vite 构建，并执行 OpenSpec 校验。

回滚方式：恢复页面导入和内联状态逻辑，删除新增 composable 文件即可；不涉及数据库或外部系统迁移。

## Open Questions

暂无。是否继续抽取页面可见性轮询、外部点击菜单和应用工作区状态，留到后续独立 change 评估。
