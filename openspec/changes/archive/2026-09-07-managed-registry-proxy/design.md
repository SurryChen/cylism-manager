## Context

`registry:2` pull-through cache 的一个实例只能可靠代理一个上游 Registry。Docker Hub 还使用特殊的认证端点，因此其上游应为 `https://registry-1.docker.io`，不能被 `registry.k8s.io` 的请求复用。

K3s 节点需要从主机侧访问稳定端点，因此使用指定节点的私网/Tailscale IP 与 NodePort，而不是 ClusterIP Service 地址。

## Decisions

### 1. 每个 Registry 使用独立代理实例

每个数据库记录拥有独立的 Kubernetes 资源名 `cylism-registry-proxy-<id>`。Deployment 的 `REGISTRY_PROXY_REMOTEURL` 来自该实例的上游配置，Service 使用该实例的 NodePort。这样 `docker.io` 与 `registry.k8s.io` 不会共享上游或缓存。

升级前的单例记录缺少 Registry、上游和资源名时，平台补齐为 Docker Hub 并识别其旧资源名 `cylism-registry-proxy`。用户可显式迁移：先删除旧 Service 释放 NodePort，再删除旧 Deployment，最后以 `cylism-registry-proxy-<id>` 重建同配置资源。入口地址和端口保持不变，但代理会短暂中断。

### 2. 受限的上游和入口配置

Registry 必须是仓库域名，入口必须是可路由的私网或 Tailscale IP。上游必须是无路径、无认证信息的 HTTPS 地址；除 Docker Hub 外，上游域名必须等于 Registry 域名，避免将一个代理误用到不兼容的上游。

创建或更新时，平台会拒绝已被另一 Registry Proxy 使用的 NodePort。

### 3. 临时缓存和安全清理

每个实例使用带大小上限的 `emptyDir`。用户触发清理或定期清理到期时，平台删除该实例标签选择的 Pod，由 Deployment 重建，以清空临时缓存。不会删除运行中 Registry 的缓存文件。

### 4. 节点镜像源独立配置

代理 Ready 后，用户在“节点镜像源”创建或更新相同 Registry 的规则，例如：

- Registry: `registry.k8s.io`
- 验证镜像: `registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.15.0`
- Endpoint: `http://<代理入口 IP>:<NodePort>`

再将该规则应用到目标节点。代理管理不会自动重写节点的 `registries.yaml`。

## Alternatives Considered

- 单个 Docker Hub 代理：无法代理 `registry.k8s.io`，未采用。
- Nginx 透明转发：无法可靠处理 Docker Registry 鉴权和 blob 重定向，未采用。
- PVC/hostPath 缓存：不符合无持久缓存目标，未采用。
