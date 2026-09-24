## Context

当前 Service 列表在返回每一项前，顺序查询该 Service 的 EndpointSlice；一次页面加载需执行 `1 + S` 次 Kubernetes API 调用，其中 `S` 是 Service 数量。配置列表则在列出 ConfigMap 或 Secret 后，按命名空间顺序查询 Deployment、StatefulSet、DaemonSet 以计算 `used_by`，一次加载需执行约 `1 + 3N` 次 Kubernetes API 调用，其中 `N` 是包含目标配置资源的命名空间数量。

页面列表只使用端点数量和引用数量来展示状态或禁用删除。这些信息不是首屏完成资源浏览所必需，端点详情和后端删除校验已经分别具备按需读取与强制保护能力。

## Goals / Non-Goals

**Goals:**

- 将 Service、ConfigMap 和 Secret 列表的 Kubernetes API 调用数量降为固定数量，不随 Service 或命名空间数量增长。
- 保留 Service 端点详情、ConfigMap/Secret 引用详情和后端删除保护。
- 避免改变现有重型 Service 列表 API 调用方的行为。
- 只在用户需要新建配置时加载命名空间选项。

**Non-Goals:**

- 不为资源 tab 增加 KeepAlive、TTL 缓存或后台预取。
- 不改变 Kubernetes RBAC、资源分页、列表排序或接口响应包装格式。
- 不移除 Service EndpointSlice 或 ConfigMap/Secret 引用的详情能力。

## Decisions

### 1. Service 通过显式参数选择轻量列表

新增 `endpoint_count=false` 查询参数。该参数存在时，handler 调用不统计 EndpointSlice 的 Service 元数据列表；默认请求继续返回 `endpoint_count`，保护既有调用方。资源页使用轻量模式，并移除“端点”列；“查看端点”继续请求单个 Service 的 EndpointSlice。

备选方案：在服务端按命名空间批量拉取所有 EndpointSlice 后汇总。该方案保留端点数量，但仍需读取全量 EndpointSlice，且对大型集群的响应体和内存成本较高；在用户允许减少展示信息的前提下不采用。

### 2. 配置列表使用已有的轻量元数据路径

资源页改用带 `usage=false` 的 ConfigMap/Secret API。该路径只列举资源元数据和键数量，跳过 `used_by` 扫描。列表移除“引用数量”列，也不再基于过期的列表引用状态禁用删除。

ConfigMap 和 Secret 的“查看”操作统一请求详情 API，以展示键及引用工作负载。删除时后端仍读取目标资源并检查工作负载和平台数据库引用，因此安全语义不变。

备选方案：在列表中并行扫描三类工作负载。它减少串行等待但依然为首屏加载不必要的全量工作负载数据，且会增加 API Server 并发压力；不采用。

### 3. 新建配置时再加载命名空间

配置页不再在挂载时读取命名空间列表。首次打开新建表单时加载并显示选择器的加载状态；读取失败时保留现有可手输命名空间的回退。

备选方案：继续首屏预加载。它实现简单，但给只浏览资源的用户增加一次无关请求；不采用。

## Risks / Trade-offs

- [列表失去端点健康概览] → 保留“查看端点”操作，用户可在需要时查看 Ready/NotReady 端点。
- [删除按钮不再提前禁用被引用资源] → 删除请求继续在服务端执行工作负载和平台引用检查，并返回明确的冲突提示。
- [首次新建时等待命名空间列表] → 仅在实际创建流程发生，且表单显示明确的加载状态和手输回退。
- [轻量 Service 参数未被调用方采用] → 默认保持现有端点统计行为，前端通过具名 API 函数明确使用轻量模式。

## Migration Plan

1. 先增加 Service 轻量查询及单元测试，默认行为不变。
2. 修改前端列表请求与展示，补充页面测试。
3. 运行后端和前端全量验证；部署后以访问日志或浏览器 Network 面板确认列表请求只触发一次 Kubernetes 列表调用。
4. 若出现用户体验问题，可将前端恢复为默认 Service 请求；后端新增参数不影响回滚。

## Open Questions

无。用户已确认不实施重复切换缓存。
