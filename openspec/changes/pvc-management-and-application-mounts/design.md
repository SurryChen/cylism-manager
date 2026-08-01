# Design: PVC 管理与应用挂载

## Data ownership

PVC 的运行态事实来源是 Kubernetes API，不新增 SQLite PVC 业务表。平台通过固定标签识别自己创建的 PVC：

- `app.kubernetes.io/managed-by=cylism-manager`
- `cylism.io/environment=environment-<environment-id>`

Environment 的 Namespace 已是全局唯一，因此 PVC 名称只需在该 Namespace 内唯一。`GET /api/k8s/pvcs?environment_id=<id>` 先解析 Environment，再只返回该 Namespace、该环境标签的 PVC，并包含绑定 PV、StorageClass、PV 回收策略和 PV node affinity 中解析出的绑定节点。接口同时返回 Kubernetes Node Name 与平台服务器自定义名称，前端显示自定义名称但提交 Kubernetes Node Name。

选择 Kubernetes 作为事实来源避免 PVC phase、容量和 StorageClass 事件复制进 SQLite 后产生漂移。

## API and resource model

新增 PVC API：

- `GET /api/k8s/pvcs?environment_id=<id>`：列出当前环境的托管 PVC、Kubernetes phase、绑定节点和回收策略。
- `POST /api/k8s/pvcs`：接收 `environment_id`、`name`、`storage` 和可选 `storage_class_name`，创建 `ReadWriteOnce` PVC。
- `DELETE /api/k8s/pvcs/:namespace/:name?environment_id=<id>`：仅删除同环境且带管理标签的 PVC；若仍被平台 Deployment 或模板引用，返回冲突而不删除。删除请求必须显式确认数据删除风险，响应和 UI 均展示绑定 PV 的 `reclaimPolicy`。

应用 `ReleaseSpec` 增加 `volumes`：

```json
[
  { "claim_name": "karakeep-data", "mount_path": "/data", "read_only": false }
]
```

模板的调度字段增加可选 `node_name`。模板仅保存 claim 名称、挂载配置与 Kubernetes Node Name，不保存 PVC 数据。渲染时为每个条目生成确定性 volume name，并写入 Pod `PersistentVolumeClaimVolumeSource` 与容器 `VolumeMount`；需要本地卷调度时将 `nodeSelector["kubernetes.io/hostname"]` 写入 PodSpec。字段名保持兼容，但不再渲染 `PodSpec.NodeName`，以免绕过 Kubernetes 调度器。

VictoriaMetrics 的安装请求同样继续接收 Kubernetes Node Name，但其 hostPath Deployment 使用相同的 hostname `nodeSelector`。当前平台没有需要保留的已安装实例，因此不保留旧 `nodeName` Deployment 的读取兼容逻辑。

## Validation and safety

模板保存与发布预检均校验：

- PVC 名称、挂载路径合法，且同一模板中的 claim 和 mount path 不重复。
- PVC 在应用 Environment 的 Namespace 中存在、带 Cylism 管理标签且标签 Environment ID 匹配。
- 写入型 PVC 与 `replicas > 1` 不兼容；首期所有 PVC 均为 RWO，因此挂载 PVC 的应用必须为单副本。
- 已 Bound 的本地 PVC 必须与模板 `node_name` 一致；未设置或选择其他节点均阻断发布。
- 对采用 `WaitForFirstConsumer` 的尚未 Bound 本地 PVC，模板必须指定节点，以确保首次绑定位置可预期。
- 非本地、无 PV node affinity 的 StorageClass 不强制节点选择，允许调度器或存储后端自行处理卷附加。
- PVC Pending 不阻止模板保存。采用 `WaitForFirstConsumer` 且已选择节点的本地 PVC 可进入首次发布，由新 Pod 触发绑定；其他 Pending PVC 在发布预检中返回明确错误，避免将 Provisioning 问题误记为 Deployment 就绪超时。

发布前仍由 Kubernetes API 重新读取 PVC，而不信任旧 Release 快照。这样 PVC 被删除、标签改变或尚未 Bound 时会安全阻断发布。

## Lifecycle

PVC 独立于 Application 和 Release 生命周期：删除应用、发布失败、回滚或模板更新都不删除 PVC。删除 PVC 仅允许用户从存储页面显式执行，且平台先检查引用关系。删除前读取并展示绑定 PV 的 `reclaimPolicy`：`Delete` 可能使 local-path 等 Provisioner 清除底层数据；`Retain` 则保留 PV 和数据等待后续人工处理。

Deployment 更新使用 `Recreate` 策略，只要模板声明了可写 PVC。此策略避免 RWO 卷在滚动更新期间同时被旧、新 Pod 挂载；无 PVC 模板保留现有 RollingUpdate 行为。

## UI

在集群导航新增“存储卷”页面：先选择项目和环境，展示 PVC 名称、容量、StorageClass、状态、绑定节点、回收策略及已被哪些应用模板使用，并支持创建和删除。未 Bound PVC 显示“首次挂载时决定节点”；本地 PVC Bound 后显示并锁定其实际节点。

在应用详情的上线模板编辑器增加“数据卷挂载”与“部署节点”区域：只列出当前应用 Environment 下的可用 PVC，允许新增/移除挂载条目、填写容器路径和只读状态。节点下拉显示平台服务器自定义名称和 K8s Node Name；选择本地 PVC 后自动填入其 Bound 节点，或要求用户为 Pending 的本地 PVC 选择一个节点。默认不添加挂载或节点约束，保证无状态应用的现有体验不变。

集群节点页在每个节点详情中展示完整标签，并提供“管理标签”操作。API 以显式 `set` 和 `remove` 集合接收变更，只对用户自定义标签调用 Kubernetes Node `Update`。`kubernetes.io/*`、`node.kubernetes.io/*`、`k3s.io/*`、`node-role.kubernetes.io/*` 及 `beta.kubernetes.io/*` 被保护，既不能新增/覆盖也不能删除；`kubernetes.io/hostname` 因为是本次调度选择器的事实来源，始终只读。标签键和值必须通过 Kubernetes label 校验。变更成功后刷新节点列表，应用模板和监控安装的节点选项仍使用 Kubernetes Node Name。

## RBAC

平台 ServiceAccount 需要 `persistentvolumeclaims` 的 `get`、`list`、`watch`、`create`、`update`、`patch`、`delete` 权限，以及 `persistentvolumes` 与 `storageclasses` 的只读权限。平台不需要 PV 或 StorageClass 的写权限。

## Alternatives

- **将 PVC 保存到 SQLite**：未采用。Kubernetes 状态与绑定容量会变化，复制存储容易产生误导。
- **允许在模板中直接输入任意 PVC 名称**：未采用。会绕过环境归属与受控资源边界。
- **首期支持 RWX 并允许多副本**：未采用。K3s 默认 local-path 通常不支持 RWX，平台不能把存储后端能力假设为通用能力。
- **用 StatefulSet 替换 Deployment**：未采用。Karakeep 等单副本应用可以先通过 RWO + Recreate Deployment 安全运行；StatefulSet 是独立能力。

## Risks and mitigations

- StorageClass 不存在或 Provisioner 失败：PVC 页面显示 phase 与 Kubernetes events，发布预检明确阻断 Pending PVC。
- 用户误删业务数据：删除入口展示回收策略和数据风险，删除前检查模板/Deployment 引用，并要求二次确认。
- 节点故障导致 hostpath PVC 不可用：页面说明当前 StorageClass；备份、迁移与高可用留给后续存储能力。
- 旧 Deployment 仍使用 PVC：删除前扫描平台 Deployment 的 volume source，防止绕过已更新模板的运行态引用。
