## Why

公共 Docker Hub 镜像站可能超时或回源失败，而集群中至少有一个节点可直接访问 Docker Hub。平台需要提供一个可被节点统一使用的自建代理，减少对公共镜像站的依赖。

用户不希望持久保存镜像缓存。Docker Distribution 的拉取代理仍需要运行期缓存，因此平台应使用临时 `emptyDir`，并安全地周期性或按容量阈值重建代理 Pod 来清空缓存。

## What Changes

- 新增平台受管的 Docker Hub Registry 代理，可指定部署节点与私网/Tailscale 可访问地址。
- 使用无 PVC 的 `emptyDir` 缓存卷、资源限制和 NodePort Service，不产生可恢复的缓存数据。
- 支持缓存大小上限、检查间隔和定期清理；达到阈值或清理周期时滚动重建代理 Pod。
- 将就绪代理地址作为 `docker.io` 节点镜像源应用到选定节点，并展示安装、缓存检查和清理状态。

## Capabilities

### New Capabilities

- `registry-proxy`: 平台管理的非持久 Docker Hub 拉取代理及其缓存清理生命周期。

## Non-goals

- 不实现通用透明 HTTP 反向代理。
- 不提供公网开放的未鉴权 Registry。
- 不持久化镜像层、不提供镜像浏览、推送或私有仓库托管。
- 第一阶段不代理 `gcr.io`、`ghcr.io`、`quay.io` 等其他上游仓库。

## Impact

- 新增 Registry 代理配置模型、REST API、Kubernetes 资源编排与后台缓存维护任务。
- 更新 `k8s/platform-deployment.yaml` 所需最小权限，复用已有 Pod exec、Deployment 与 Pod 管理能力。
- 集群镜像源页新增代理配置与状态界面。
