## Why

阶段一至四已将交付与发布领域的 HTTP 编排下沉到 Service/Kubernetes Adapter，但服务器、节点、存储和网络能力仍集中在 `internal/api`。这些 Handler 同时持有 Gin、Store、SSH 和 Kubernetes 生命周期逻辑，令同一基础设施能力难以被 Web、CLI、Agent 和后台任务稳定复用。

## What Changes

- 按服务器、节点、存储、网络四个领域迁移基础设施 HTTP Adapter 至 `internal/api/infrastructure/`，保留全部既有 REST 路径、鉴权和响应契约。
- 为服务器与节点操作建立 Cluster Service 边界；为 PVC、导入和迁移建立 Storage Service 边界；为域名、证书、Ingress 与集群 DNS 建立 Network Service 边界。
- 将跨领域复用的 Kubernetes 操作收敛到 `internal/k8s`，保持 SSH/Agent 等具体基础设施适配由路由注入。
- 以现有 Handler 测试为契约，按领域逐一迁移并完成全量回归；不在本阶段改变页面行为或 API 功能。

## Capabilities

### New Capabilities

- `infrastructure-api-services`: 基础设施 API 的分层边界、依赖注入和兼容性要求。

### Modified Capabilities

- None.

## Impact

- 受影响代码为 `internal/api/server_handler.go`、`node_handler.go`、PVC 相关 Handler、域名/证书/Ingress/DNS Handler、`internal/k8s` 及其测试与路由注册。
- 现有 `/api/servers`、`/api/nodes`、PVC、域名、证书、Ingress 和 DNS REST 接口保持不变。
- 不新增第三方依赖、数据库迁移或前端接口变更。
