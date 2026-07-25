## Context

Cylism Manager 已通过 `internal/k8s/client.go` 建立了 K3s 集群连接（InCluster + 环境变量 + localhost fallback），并实现了 Node、Traefik IngressRoute、cert-manager Certificate 的基础管理。当前 `k8s_handler.go` 已有 Pod/Service/Deployment 的简单列表接口，但前端未消费，且缺少变更操作（扩缩容、镜像更新、回滚）以及 StatefulSet、DaemonSet、ConfigMap、Secret、EndpointSlice、标准 Ingress 的支持。

本次设计在前述 brainstorming 决策基础上，完善技术方案。

## Goals / Non-Goals

**Goals:**
- 实现 Deployment/StatefulSet/DaemonSet 全生命周期管控（CRUD + 变更操作）
- 实现 Service → EndpointSlice → Endpoint 映射链路展示
- 实现 ConfigMap/Secret 查看（Secret 遮蔽）及关联工作负载追踪
- 实现标准 K8s Ingress CRUD，与 Traefik IngressRoute 并行，检测并标注 Controller
- Dashboard 增强 Deployment/Service 统计
- 所有数据实时从 K8s API 拉取，不持久化

**Non-Goals:**
- 不持久化 K8s 资源到 SQLite（K8s 是唯一权威来源）
- 不实现 ConfigMap/Secret 的创建和编辑（保持安全边界，先只读）
- 不实现 Pod 日志流式拉取（留待后续）
- 不实现 Traefik IngressRoute 创建/编辑（现有标记"待实现"，本次也暂不实现）
- 不实现 RBAC/多租户（现有架构单用户）

## Decisions

### 决策 1: 后端架构 — 按资源拆分文件，handler 集中

**选择**: 在 `internal/k8s/` 下新增 `workload.go`、`service.go`、`config.go`、`ingress_std.go` 四个文件，每个文件封装对应资源的客户端操作。所有 HTTP handler 集中在 `internal/api/k8s_handler.go`（扩展现有文件）。

**备选**: 按 handler 拆分 `api/` 下多个文件（如 `workload_handler.go`）。  
**取舍**: 当前 handler 方法较少（约 20 个），集中管理减少文件碎片；未来超过 50 个方法时再拆分。

### 决策 2: 变更操作使用 PATCH 合并更新

**选择**: 扩缩容和镜像更新使用 `PATCH`（JSON Merge Patch 或 Strategic Merge Patch），而非全量 PUT。

**备选**: 全量 PUT 替换资源。  
**取舍**: PATCH 避免并发冲突和意外覆盖其他字段（如经 Ingress Controller 修改的 annotation），更安全。Go 端使用 `k8s.io/client-go` 的 `Deployments(namespace).Patch()` 方法。

### 决策 3: EndpointSlice 优先，自动 fallback

**选择**: 优先查询 `discovery.k8s.io/v1` EndpointSlice；若 API 不可用，fallback 到传统 `v1/Endpoints`。后端统一封装为 `EndpointInfo` 结构返回前端。

**备选**: 只支持 EndpointSlice，不 fallback。  
**取舍**: K3s 不同版本行为差异大（v1.21 前无 EndpointSlice），fallback 保证兼容性。

### 决策 4: 前端导航 — 基础设施二级分组

**选择**: 在现有侧边栏"基础设施"分组内新增"工作负载""服务发现""配置"三个一级入口；路由页（现有 `/routes`）扩展双 Tab。

**备选**: 新建独立"K3s 集群"顶级分组。  
**取舍**: 服务器管理本质上属于基础设施范畴，将 K3s 资源管理与服务器管理放在同一分组下符合运维心智模型，且避免侧边栏分组过多。

### 决策 5: 配置页不实现创建/编辑

**选择**: ConfigMap/Secret 只提供查看功能（列表 + 详情），不提供创建、编辑、删除。

**备选**: 完整 CRUD。  
**取舍**: Secret 编辑涉及 base64 编解码校验和敏感数据安全，风险较高；ConfigMap 编辑可能意外影响正在运行的工作负载。先做安全边界最小的只读模式。

### 决策 6: Ingress Controller 检测

**选择**: 页面加载时调用现有 `/api/system/crds` 检测 `ingressroutes.traefik.io` CRD 是否存在；同时查询 `ingress-nginx` 命名空间下的 Deployment 状态。在路由页顶部 Banner 展示："Ingress Controller: Traefik vX.X（已检测）"。

**备选**: 只检测 CRD 不检测 Deployment 状态。  
**取舍**: CRD 存在不等于 Controller 运行中。组合检测 CRD + Deployment 状态能准确判断流量管理是否可用。

## Risks / Trade-offs

- **[K8s 版本兼容]** K3s 不同版本的 API 行为差异（EndpointSlice 可用性、Ingress API 版本 v1 vs v1beta1） → 运行时检测 API Group/Version 并 fallback
- **[扩缩容并发]** 多人同时对同一 Deployment 扩缩容可能冲突 → 前端展示当前副本数，PATCH 操作失败时提示刷新重试
- **[Secret 泄露]** Secret value 在前端虽然遮蔽展示，但通过网络传输未加密 → API 已受 JWT 保护；Secret endpoint 额外增加审计日志
- **[性能]** 列表页每次刷新全量拉取集群资源 → 当前面向小集群（<100 资源）无瓶颈；未来可加服务端分页/缓存
- **[导航膨胀]** 新增 3 个一级入口加重侧边栏 → 现有折叠分组设计可承载；需观察用户反馈

## Migration Plan

无需数据迁移（不持久化 K8s 资源到 SQLite）。部署步骤：

1. 部署新版本后端二进制，自动注册新路由
2. 部署新前端静态文件，`vite build` 后替换 `web/dist/`
3. 无需 K8s 集群侧变更（纯 API 读取 + PATCH 操作）

回滚：部署旧版本二进制和前端静态文件即可，无数据库 schema 变更。
