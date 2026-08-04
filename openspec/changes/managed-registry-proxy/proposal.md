## Why

不同 Registry 的网络可达性并不相同。例如节点可以访问 Docker Hub，但无法直接访问 `registry.k8s.io` 实际跳转的上游。单个固定 Docker Hub 代理不能代理 Kubernetes 官方镜像，且错误地把其端点配置给其他 Registry 会导致镜像不存在。

平台需要将自建 Registry Proxy 扩展为多个实例。每个实例明确绑定一个 Registry 和一个受控的 HTTPS 上游，节点镜像源再按 Registry 选择对应实例。

## What Changes

- 支持创建、查看、更新和清理多个平台受管 Registry Proxy 实例。
- 每个实例配置名称、目标 Registry、上游地址、部署节点、私网/Tailscale 入口、NodePort 与缓存生命周期。
- Docker Hub 作为默认预设，留空上游时使用 `https://registry-1.docker.io`；其他 Registry 默认使用自身 HTTPS 地址。
- 每个实例创建独立 Deployment、Service 与 `emptyDir` 缓存，拒绝重复 NodePort。
- 保留原有 Docker Hub 单例接口与资源名兼容性，已有实例升级后仍会被识别为 Docker Hub 代理。

## Capabilities

### New Capabilities

- `registry-proxy`: 平台管理的非持久、多上游 Registry 拉取代理及其缓存清理生命周期。

## Non-goals

- 不实现通用透明 HTTP 反向代理。
- 不提供公网开放的未鉴权 Registry。
- 不持久化镜像层、不提供镜像浏览、推送或私有仓库托管。
- 不自动修改节点镜像源；用户确认代理就绪后选择对应 Registry 的镜像源并应用节点。

## Impact

- 扩展 Registry 代理模型、REST API、Kubernetes 资源编排与集群镜像源页面。
- `kube-state-metrics` 保持 Kubernetes 官方 `registry.k8s.io` 镜像地址，不再错误替换为 Docker Hub 地址。
