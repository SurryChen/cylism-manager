## Context

当前 K3s 集群由一个控制平面和三个 Agent 节点组成，并提供节点本地的 `local-path` StorageClass。Cylism Manager 使用 SQLite；尚未具备共享存储、内网 DNS、内网 CA 或外部 OCI Registry。现有 `RegistryProxy` 是无持久化的拉取缓存，`ImageRegistry` 管理发布时可使用的 Registry 凭据，`NodeRegistryMirror` 能通过受控 SSH 写入各节点的 `/etc/rancher/k3s/registries.yaml` 并重启 K3s。

本变更只实现交付链路的制品落点。源码归档上传、Runner 领取任务、BuildKit 构建和镜像浏览/清理将在后续独立变更中完成。

## Goals / Non-Goals

**Goals:**

- 由 Cylism Manager 在 K3s 中创建、更新、观察和删除单个持久 OCI Registry。
- 通过 `local-path` PVC 将 Registry 数据固定在用户选择的数据节点，避免 Pod 调度到其他节点后得到空仓库。
- 允许用户配置 Registry 的 CPU/内存 requests 与 limits，并使用保守默认值。
- 为 Registry 创建 Kubernetes Basic Auth Secret，凭据明文只在创建或显式轮换时出现一次。
- 复用 `ImageRegistry` 与 `NodeRegistryMirror`，将 Registry endpoint 和拉取凭据以受控方式应用至选定 K3s 节点。
- 通过 UI 明确展示 endpoint、TLS 模式、节点应用进度、风险与运行状态。

**Non-Goals:**

- 不自行实现 OCI Distribution API、镜像层存储、tag 浏览、删除、垃圾回收或 Harbor 功能。
- 不实现源码上传、CI Runner、BuildKit、Git/Webhook、SBOM、扫描、签名或自动发布。
- 不实现 Registry 高可用、跨节点故障切换、共享卷、证书签发或内网 DNS 管理。
- 不自动将 HTTP/insecure 模式应用到任何节点；用户必须选择节点并确认风险。

## Decisions

### 1. 使用 `registry:2`，而不是在 Manager 中实现 Registry 协议

平台创建 `registry:2` Deployment、ClusterIP Service 和 host-based Ingress。Distribution 已具备 OCI/Docker V2 API、blob 上传、manifest、认证和一致性语义；Manager 只控制其生命周期并保存元数据。

**Alternatives considered:** 自研 OCI Registry 会扩大安全边界，涉及上传会话、分层存储、垃圾回收和兼容性，未采用。Harbor 是后续规模化替代方案，但对当前单机存储场景过重。

### 2. 单副本、节点固定的 PVC 持久存储

Registry 使用单副本 `Recreate` 策略、`local-path` StorageClass 的 `ReadWriteOnce` PVC 和 `nodeSelector`。创建时用户选择数据节点和容量，容量默认 `100Gi`；平台在目标 namespace 创建名为 `cylism-oci-registry-data` 的 PVC，并将其挂载到 `/var/lib/registry`。PVC 必须使用 `WaitForFirstConsumer`，让调度器先依据 `nodeSelector` 选择节点，再由 local-path Provisioner 在该节点创建本地 PV。

平台在创建前读取 `local-path` StorageClass 并要求其存在且 `volumeBindingMode=WaitForFirstConsumer`。不满足时拒绝创建，避免 PVC 先绑定到未选择节点或永久 Pending。删除 Registry 资源时不删除 PVC；若同名 PVC 已存在，平台仅在其标签、StorageClass、访问模式与请求容量满足受管 Registry 要求时复用，否则拒绝接管。数据节点、StorageClass 与 PVC 容量在创建后不可变，避免本地卷迁移被误认为普通配置更新。

**Alternatives considered:** 直接 `hostPath` 虽能固定目录，但没有 PVC 生命周期、容量声明和集群资源可见性，因此不采用。共享块/文件存储等待基础设施具备后再支持。

### 3. 明确的运行资源配置

创建和更新请求保存 Registry 容器的 CPU/内存 requests 与 limits。默认值为 CPU request `100m`、CPU limit `500m`、内存 request `256Mi`、内存 limit `1Gi`；用户可编辑四项值。API 校验 Kubernetes quantity 格式、所有值为正数，并要求每个 request 不超过对应 limit。Deployment 始终写入这四个资源值，避免 Registry 以 BestEffort QoS 与业务工作负载无边界争抢资源。

### 4. endpoint 必须是无路径的 hostname 或 host:port

Registry 客户端以 Registry authority 识别仓库，不能安全地复用应用的 URL path。因此 Manager 仅接受 `registry.example.internal`、`registry.example.internal:5443` 或 `10.32.160.103:80` 形式的 endpoint；拒绝 URL path、用户信息和 query。

HTTPS endpoint 要求用户从平台证书管理中选择同一命名空间内、状态为 Ready 且覆盖 Registry hostname 的证书；平台读取该 Certificate 的 `secretName` 并让 Ingress 引用同命名空间 TLS Secret，不允许手工提交 Secret 名称或跨命名空间复制私钥。HTTP endpoint 必须显式设置 `insecure_http=true`，UI 和 API 返回持久风险提示，并仅在用户选择节点后写入 containerd 配置。首期不为 HTTP 默认静默启用。

**Alternatives considered:** 用 `/registry` 路径共享现有 Ingress 会破坏 Docker Registry 的上传重定向与客户端 URL 语义，未采用。默认 HTTP 会导致节点无意间信任明文凭据，未采用。

### 5. 认证与现有模型复用

创建时由用户提交初始 `pull_username` 和 `pull_password`，Manager 在目标 namespace 生成 htpasswd Secret，配置 Registry Basic Auth；明文不会存入数据库。使用平台加密密钥保存 Node Registry Mirror / ImageRegistry 所需的拉取凭据，所有 API 响应仅返回 `credential_configured`。

受管 Registry 成功创建后，Manager 创建或更新一个具名 `ImageRegistry`，使项目授权和现有发布校验保持一致。还创建或更新一个 `NodeRegistryMirror`，将 endpoint、认证和 HTTP/TLS 策略分发给用户选择的节点。该同步关联为一对一，不能覆盖同 endpoint 但非平台托管的记录。

**Alternatives considered:** 匿名 Registry 无法保护内网制品，未采用。为每个项目创建独立 Registry 或凭据属于后续多租户设计，未包含。

### 6. 管理面与运行时资源边界

新 `ManagedOCIRegistry` 表只保存期望配置、资源名、关联的 `ImageRegistry`/`NodeRegistryMirror` ID、状态和脱敏错误。Kubernetes 是运行状态真实来源。资源固定带 `app.kubernetes.io/managed-by: cylism-manager` 和 `cylism.io/managed-registry` 标签；创建时发现同名但非受管资源必须阻断。

所有修改均记录现有操作日志。部署完成后通过 Service endpoint 和 Registry `/v2/` 返回的预期 `401` 验证认证链路；不记录请求中的 Authorization 值。

## Risks / Trade-offs

- [`local-path` PVC 仍受单节点磁盘与节点可用性限制] → 固定节点、显示 PVC 与节点状态、保留 PVC 并提供备份指引；后续引入共享持久卷。
- [StorageClass 未启用 `WaitForFirstConsumer` 时，PVC 可能不在所选节点创建] → 部署前严格校验并拒绝不满足条件的 StorageClass。
- [HTTP endpoint 可泄露镜像与 Basic Auth 凭据] → 默认拒绝，要求明确确认，限定选择节点并在所有状态/API 中标记不安全。
- [错误更新 `registries.yaml` 会影响节点拉取镜像] → 复用逐节点异步应用和状态记录，创建前校验 endpoint 唯一性，失败时保留原状态并提供重试。
- [`registry:2` 镜像在完全离线环境无法拉取] → UI 要求输入可拉取的 Registry image reference，并在部署前检验；可由管理员预先导入该镜像。
- [删除 Registry 会破坏已有工作负载拉取和历史 Release] → 删除必须先检查关联 Release/默认项目仓库，阻断或要求先解除引用；永不自动删除数据目录。

## Migration Plan

1. AutoMigrate 为受管 Registry 增加 PVC 名称、StorageClass、请求容量和容器资源字段；不修改现有 Registry Proxy、ImageRegistry 或 NodeRegistryMirror 数据。
2. 已存在的 hostPath Registry 在升级后标记为需要迁移，不会自动创建或复制 PVC 数据。新建 Registry 只创建 PVC、Kubernetes Secret、Deployment、Service、Ingress，以及受控关联的仓库/节点镜像源记录。
3. 用户选择节点后异步写入对应 `registries.yaml`；每个节点记录独立成功/失败状态。
4. 回滚部署时删除 Manager 创建的 Kubernetes 工作负载资源和受控关联记录，但不删除 PVC；已应用节点配置须由用户通过受管入口解除。

## Open Questions

- 首次实际部署应使用哪个内网 hostname 和 TLS Secret；在未提供前，仅允许以明确确认的 HTTP endpoint 运行。
- HTTPS 证书必须先在“网络管理 → 证书”中创建，且证书命名空间与 Registry 命名空间一致；Registry 页面只展示匹配 hostname 且 Ready 的证书。
- Registry PVC 的备份位置、频率和保留周期由运维策略确定，首期只展示 PVC、节点与风险。
