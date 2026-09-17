## Why

交付中心的制品库页面需要由规范 query Tab 表示当前工作区；同时自托管制品库将低频服务维护和高频镜像目录操作堆叠在一条没有层级的流程中，导致操作入口、搜索和空白标签面板难以理解。Proxy 实例卡也没有遵循同页受管 Registry 的玻璃卡表面，并将健康诊断摘要误呈现为红色错误。

## What Changes

- 将自托管制品库与 Registry Proxy 工作区设为同一聚合页下的规范 Tab URL：`/delivery/registry?tab=registry` 与 `/delivery/registry?tab=registry-proxy`。
- 让两个 Tab 始终由当前 Vue Router 查询参数派生，确保点击、刷新、复制地址和浏览器历史一致。
- 让 Registry Proxy 实例卡使用与自托管制品库指标卡一致的玻璃表面、边框、阴影、圆角和模糊效果，同时保留实例内部属性网格的细分隔线。
- 重组自托管制品库为“运行概览”和“镜像目录”两个连续但职责清晰的操作区；将服务动作归入概览，将目录标题、数量和搜索放在同一操作栏，并在初次读取目录后自动显示首个仓库的标签。
- 移除 Proxy 工作区中重复的 `Registry Proxy` 标题；将上游诊断作为属性网格的“上游连通性”单元呈现，只有实际的 Proxy 生命周期错误才使用独立错误提示。

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `registry-proxy`: Proxy 工作区的规范入口和实例列表视觉表面变更。
- `managed-oci-registry`: 自托管 Registry 控制台的信息架构和默认目录选择行为变更。
- `ui-navigation`: 交付中心制品库工作区改为独立的可直达路由，并兼容历史查询地址。

## Impact

- 修改 `ManagedOCIRegistries.vue` 的路由集成、目录操作布局和关联视图测试。
- 修改 `RegistryProxyWorkspace.vue` 与其组件测试；不改变 `/registry-proxies` REST API、后端、持久化数据或 Proxy Kubernetes 资源。
