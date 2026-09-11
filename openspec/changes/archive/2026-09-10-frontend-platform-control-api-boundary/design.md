## Context

此前的前端治理已经将应用、监控和基础设施页面的大部分 REST 调用收敛到 `web/src/api/`，并采用 `useAsyncResource` 处理可重叠读取。平台控制面仍有一批直接调用：系统组件的配置/恢复、平台入口和发布设置、临时访问令牌以及 Cluster DNS 策略。这些操作具有高权限和较高影响面，但缺少统一的 API contract 测试。

## Goals / Non-Goals

**Goals:**

- 让每个控制面 endpoint 由一个命名 API 模块拥有，业务参数在前、可选 request options 在后。
- 保持 HTTP method、路径、URL 编码、payload 和已解包返回值不变；未传 options 时保持现有调用形态。
- 将可重叠的控制面读取接入 `useAsyncResource`，避免旧响应或卸载后的响应更新页面。
- 将 mutation 失败显示在最小相关区域，保留原有表单、确认框和已加载数据以便修正或重试。

**Non-Goals:**

- 不修改 Go 后端、Kubernetes/Helm/DNS 业务工作流、发布确认语义或轮询策略。
- 不拆分现有系统设置子页面、系统组件页面或 Cluster DNS 页面，也不引入全局状态、错误总线或新的 UI 框架。
- 不将 `RuntimeManagement.vue`、ChatDrawer、Chart 仓库或镜像仓库纳入本 change。

## Decisions

### 领域 API 模块按已有边界扩展

`system-components.js` 继续拥有系统组件操作；`settings.js` 继续拥有平台入口、发布与临时访问令牌操作；Cluster DNS 新建小型 `cluster-dns.js`，避免将 DNS 策略混入泛化的 Kubernetes API 模块。

备选方案是在页面内保留请求或将 DNS 放入 `kubernetes.js`。前者继续分散请求契约；后者会把具有独立策略与历史回滚语义的 DNS 工作流隐藏在过宽的通用模块中，因此不采用。

### API wrapper 保持原协议和调用形态

每个 wrapper 只转发既有请求，必要时编码路径段。无 request options 时不额外传递 `undefined`，以保持现有调用方和测试 mock 的参数形态；有 options 时转发 `AbortSignal`。

### 读取由页面维持所有权

读取不进入全局 store。`useAsyncResource` 继续由拥有页面维护，在新的读取开始、目标变化或组件卸载时取消或忽略旧请求。系统设置已有的独立子区域资源保持独立，Cluster DNS 使用自己的资源，避免 DNS 失败遮蔽其他控制面状态。

### mutation 错误保持在操作表面

配置保存、恢复、入口接管/修复、发布、令牌和 DNS 操作失败时，保持对应 modal、表单或确认状态，并显示局部错误；成功后仍沿用原有刷新路径。

## Risks / Trade-offs

- [将已有调用迁移到 wrapper 时错改参数顺序] → 先写 API 合同测试，覆盖 method、path、payload、编码与 options。
- [DNS 或发布读取在切换/关闭时产生旧状态] → 使用 `useAsyncResource`，并补 deferred request、abort 与 unmount 测试。
- [局部错误重复或掩盖已加载数据] → 每个 workflow 使用独立 error ref；读取错误和 mutation 错误不共享清空逻辑。
- [测试 mock 依赖无 options 调用形态] → wrapper 在未传 options 时不增加多余实参，并保留现有 mock 兼容性。

## Migration Plan

1. 先为新增 API 函数建立 contract tests，再实现薄 wrapper。
2. 逐页迁移调用并补局部错误、请求取消与回归测试。
3. 执行前端、后端、构建和 OpenSpec 全量验证。
4. 无数据或后端迁移；出现前端回归时可回退本次页面/API wrapper 改动。

## Open Questions

- 无。Runtime 管理将在后续独立 change 中处理。
