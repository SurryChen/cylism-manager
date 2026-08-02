## Context

`registry:2` 的 pull-through cache 代理 Docker Hub 时必须使用本地 storage 保存运行期 manifest 与 blob。完全无状态的 HTTP 反向代理会遇到 Docker Hub token、重定向与 CDN 链路，无法可靠作为 K3s 的镜像源。

平台已能通过 SSH 向节点写入 `registries.yaml`，并具有 `pods/exec`、Deployment 和 Pod 的管理权限。K3s 节点需要一个主机侧可访问的稳定端点，Kubernetes ClusterIP 域名不能作为宿主机镜像拉取端点。

## Decisions

### 1. Registry Distribution 临时缓存代理

平台在指定节点部署 `registry:2`，配置 `REGISTRY_PROXY_REMOTEURL=https://registry-1.docker.io`。缓存存储在 `emptyDir`，不绑定 PVC 或 hostPath；Pod 被替换后全部缓存自动删除。

该选择保留完整 Docker Registry v2 协议与 Docker Hub 鉴权流程，避免节点直接依赖不稳定的第三方镜像站。

### 2. 受控 NodePort 私网暴露

代理使用固定范围内的 NodePort 并要求用户提供节点间可访问的内网或 Tailscale 地址。平台将该地址写入 `docker.io` 镜像源配置。UI 明确提示必须使用私网地址，并拒绝公网地址、环回地址和空地址。

NodePort 允许目标节点外的 K3s 节点访问代理。网络隔离由宿主机防火墙或 Tailnet ACL 负责；平台不将它配置为 Ingress 或公网域名。

### 3. 安全的缓存清理采用 Pod 重建

平台按配置的检查间隔通过 `pods/exec` 在代理容器内执行只读的 `du -sb /var/lib/registry`。当缓存达到阈值，或距上次清理超过周期，平台删除代理 Pod，由 Deployment 重建。`emptyDir` 随旧 Pod 删除，避免并发删除 Registry 内部文件。

硬性 `emptyDir.sizeLimit` 作为最后保护；检查阈值必须小于该上限。清理期间节点可能短暂回源 Docker Hub，K3s 默认镜像源回退保持可用。

### 4. 先部署代理，再应用节点镜像源

创建流程先应用 Deployment/Service 并等待代理 Ready，然后写入节点 `registries.yaml`。这样代理镜像首次拉取不会形成对自身的镜像源循环。若代理不可用，K3s 保留 Docker Hub 默认端点作为回退。

## Alternatives Considered

- Nginx 透明转发：无缓存但无法可靠处理 Docker Hub 认证和 blob CDN 重定向，未采用。
- PVC/hostPath 缓存：可提高命中率，但不符合无持久数据目标。
- 直接删除 Registry cache 文件：可能破坏正在读取或写入的 Registry 索引，未采用。
- Harbor Proxy Cache：功能更完整，但资源开销明显高于当前需求。

## Risks and Mitigations

- 清理产生短暂不可用：仅删除单副本 Pod，K3s 回退原始 Docker Hub；UI 记录清理原因与时间。
- NodePort 被公网访问：限制为私网/Tailscale 地址并在 UI 提示防火墙要求。
- 上游不可达：代理状态展示 Docker Hub 连通性；不将“代理已部署”误报为可拉取。
- exec 查询失败：保留运行状态并记录检查失败，不执行盲目清理。
