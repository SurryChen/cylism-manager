## Why

当前内网环境只能人工导入 OCI 镜像 tar，无法给后续的源码包构建、CI Runner 推送和按 digest 发布提供稳定制品落点。现有 Registry Proxy 仅缓存上游拉取内容，既不保存镜像也不接受推送，不能替代私有 Registry。

## What Changes

- 新增由平台部署和管理的持久化 OCI Registry，使用 Docker Distribution `registry:2`、命名空间内的 `local-path` PVC，并显示部署、资源与存储状态。
- 创建表单支持编辑 Registry 的 CPU/内存 requests 与 limits；默认预填 CPU `100m`/`500m`、内存 `256Mi`/`1Gi`。
- Registry 保留用户选择的数据节点。平台使用 `WaitForFirstConsumer` 的 `local-path` PVC，使本地 PV 在该节点上创建，而不再直接挂载节点目录。
- 为受管 Registry 创建受限 Basic Auth 凭据，保存为 Kubernetes Secret，并将拉取配置分发至所选 K3s 节点。
- 将 Registry 同步为既有项目可授权的 `ImageRegistry`，使应用发布可选择其稳定 endpoint。
- 提供自托管制品库入口和 Registry 详情，显示 endpoint、运行资源、PVC、节点应用状态和必要的人工操作说明。
- 支持 HTTPS hostname endpoint；仅在用户明确确认后支持 HTTP endpoint，并将节点风险和重启影响明确展示。
- HTTPS 证书从平台证书管理中选择，仅允许使用目标命名空间内、状态为 Ready 且覆盖 Registry 域名的证书；不再让用户手填 TLS Secret。

## Capabilities

### New Capabilities

- `managed-oci-registry`: 部署、保护、观察和卸载平台受管的持久 OCI Registry，并将其配置分发给 K3s 节点。

### Modified Capabilities

- `ui-navigation`: 增加交付中心的导航入口。

## Impact

- 新增 Registry 数据模型、SQLite migration、REST API、Kubernetes Deployment/Service/Ingress/Secret/PVC 渲染和状态探测。
- 扩展现有镜像仓库和节点镜像源逻辑，新增对平台托管 Registry 的受控关联。
- 新增 Vue 自托管制品库配置页面及 API 测试；Registry 运行时依赖 `registry:2` 镜像、可用的 `local-path` StorageClass、所选节点和可达 endpoint。
