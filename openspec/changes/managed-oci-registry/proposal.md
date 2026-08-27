## Why

当前内网环境只能人工导入 OCI 镜像 tar，无法给后续的源码包构建、CI Runner 推送和按 digest 发布提供稳定制品落点。现有 Registry Proxy 仅缓存上游拉取内容，既不保存镜像也不接受推送，不能替代私有 Registry。

## What Changes

- 新增由平台部署和管理的持久化 OCI Registry，使用 Docker Distribution `registry:2`，并显示部署与存储状态。
- 为受管 Registry 创建受限 Basic Auth 凭据，保存为 Kubernetes Secret，并将拉取配置分发至所选 K3s 节点。
- 将 Registry 同步为既有项目可授权的 `ImageRegistry`，使应用发布可选择其稳定 endpoint。
- 提供“交付中心”入口和 Registry 详情，显示 endpoint、运行状态、节点应用状态和必要的人工操作说明。
- 支持 HTTPS hostname endpoint；仅在用户明确确认后支持 HTTP endpoint，并将节点风险和重启影响明确展示。

## Capabilities

### New Capabilities

- `managed-oci-registry`: 部署、保护、观察和卸载平台受管的持久 OCI Registry，并将其配置分发给 K3s 节点。

### Modified Capabilities

- `ui-navigation`: 增加交付中心的导航入口。

## Impact

- 新增 Registry 数据模型、SQLite migration、REST API、Kubernetes Deployment/Service/Ingress/Secret 渲染和状态探测。
- 扩展现有镜像仓库和节点镜像源逻辑，新增对平台托管 Registry 的受控关联。
- 新增 Vue 交付中心与 Registry 配置页面及 API 测试；Registry 运行时依赖 `registry:2` 镜像、节点可写的持久化目录和可达 endpoint。
