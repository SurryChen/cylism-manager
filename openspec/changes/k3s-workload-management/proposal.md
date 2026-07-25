## Why

Cylism Manager 已具备 K8s 集群连接能力和基础的节点/IngressRoute 管理，但缺少对工作负载（Deployment/StatefulSet/DaemonSet）、服务发现（Service/EndpointSlice）和配置（ConfigMap/Secret）的全生命周期管控。运维人员当前需要切换 kubectl CLI 来完成扩缩容、镜像更新、回滚、服务排查等日常操作，无法在一个面板内闭环。本次变更补齐 K3s 集群管控的核心缺失能力。

## What Changes

- 新增"工作负载"页面，支持 Deployment/StatefulSet/DaemonSet 的列表查看、扩缩容、镜像更新、版本回滚、关联 Pod 查看
- 新增"服务发现"页面，展示 Service 与 EndpointSlice/Endpoint 的映射链路，辅助排查服务连通性问题
- 新增"配置"页面，管理 ConfigMap 和 Secret，支持查看键值对（Secret 遮蔽）、追踪被哪些工作负载引用
- 扩展"路由"页面，增加标准 K8s Ingress Tab 与 Traefik IngressRoute 并行，检测并标注当前使用的 Ingress Controller
- Dashboard 增强：集群概况 Strip 新增 Deployment/Service 统计
- **BREAKING**: 无。所有新增 API 和页面均为增量添加，不修改现有接口

## Capabilities

### New Capabilities

- `k3s-workload`: Deployment/StatefulSet/DaemonSet 全生命周期管理（列表、详情、扩缩容、更新镜像、回滚、关联 Pod）
- `k3s-service-discovery`: Service 列表/详情 + EndpointSlice/Endpoint 查询，Service→Endpoint 映射链路展示
- `k3s-config`: ConfigMap/Secret 列表/详情查看，Secret 遮蔽展示，关联工作负载追踪
- `k3s-ingress-std`: 标准 K8s Ingress 的 CRUD，与现有 Traefik IngressRoute 并行，Ingress Controller 状态感知

### Modified Capabilities

- `ingressroute-crud`: 新增 Traefik Controller 检测标记，路由页增加 Ingress Tab
- `k8s-dashboard`: Dashboard 摘要增加 Deployment/Service 统计指标

## Impact

- **后端**: `internal/k8s/` 新增 4 个源文件（workload.go, service.go, config.go, ingress_std.go），`internal/api/k8s_handler.go` 扩展约 20 个 handler 方法，`internal/api/routes.go` 注册新路由
- **前端**: 新增 3 个视图页面（Workloads.vue, Services.vue, Configs.vue），扩展现有 Sites.vue（路由页增加 Ingress Tab + Traefik 检测），Dashboard.vue 增加统计卡片
- **路由**: `web/src/router/index.js` 新增 3 条路由
- **导航**: `web/src/App.vue` 基础设施分组新增 3 个入口
- **依赖**: 无新增第三方依赖，全部使用现有 `k8s.io/client-go`
