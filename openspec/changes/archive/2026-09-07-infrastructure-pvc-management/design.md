## Context

平台在 `monitoring` 命名空间运行单副本 VictoriaMetrics、vmalert 和 Alertmanager。Alertmanager 已使用名为 `cylism-alertmanager-data` 的 `ReadWriteOnce` PVC，但其所有权仅能从标签推断。VictoriaMetrics 使用用户填写的 `hostPath`，无法由存储页统一管理，也使节点目录成为配置契约。当前集群多使用 K3s local-path provisioner，该卷会在首次被带节点约束的 Pod 使用时绑定到对应节点。

## Goals / Non-Goals

**Goals:**

- 为新 VictoriaMetrics 安装创建平台托管的 PVC，并固定挂载到容器内 `/storage`。
- 让 Alertmanager 和 VictoriaMetrics 的持久卷在基础设施存储库存中可见、可识别且不可被通用应用卷操作破坏。
- 在 VictoriaMetrics PVC 首次绑定前用 `kubernetes.io/hostname` nodeSelector 约束 Pod 调度，展示实际绑定节点和 StorageClass。
- 明确识别旧 hostPath 安装，支持将其历史数据受控迁移到系统管理 PVC，且避免升级或修改保留天数时丢失数据。

**Non-Goals:**

- 不在普通安装或配置更新中隐式迁移 hostPath 数据，不自动删除旧目录，也不在本变更将数据跨节点迁移。
- 不提供基础设施 PVC 的通用创建、修改、扩容、备份、导入、删除或迁移入口。
- 不实现多副本 HA VictoriaMetrics、RWX 存储、对象存储备份或用户自定义 PV 路径。

## Decisions

### 以专用 PVC 作为 VictoriaMetrics 的默认持久化介质

新安装创建 `monitoring/cylism-victoria-metrics-data`，带平台管理标签及 `cylism.io/infrastructure=victoria-metrics` 标识；PVC 为单副本、`ReadWriteOnce`，容器始终将其挂载至 `/storage`。安装表单从“宿主机路径”改为“容量 + StorageClass”，默认容量为 `10Gi`，允许选择已发现的 StorageClass 或使用集群默认类。

Deployment 继续使用 `kubernetes.io/hostname` nodeSelector。对于 `WaitForFirstConsumer` local-path PVC，这使 PVC 与 VictoriaMetrics Pod 在用户选择的节点首次绑定，避免 PVC 绑定到另一节点后 Pod 无法调度。

选择 PVC 而非继续暴露 hostPath，是为了将容量、回收策略、绑定节点和使用方纳入 Kubernetes 的可观测对象。允许任意 hostPath 虽灵活，但无法安全纳入现有存储工作流。

### 基础设施所有权从 Kubernetes 标签推断

不新增 SQLite 表。PVC API 根据命名空间、平台管理标签和基础设施标签返回 `owner_type`、`owner_name`、`managed_operations` 等派生字段。Alertmanager 的现有 PVC 在 reconcile 时补齐相同标签；VictoriaMetrics 新 PVC 从创建时就带标签。

这让实际 K8s 资源始终是事实来源，避免数据库与已卸载或手工恢复的资源漂移。存储页按“应用 / 基础设施 / 外部”展示和筛选；基础设施行只提供跳转到监控设置，不显示通用破坏性操作。

### 对旧 hostPath 实例采用显式、同节点迁移

状态读取同时识别 PVC 和 hostPath 两种 Deployment volume。若识别到 hostPath，监控页将其标示为“旧宿主机目录存储”，配置更新仅允许保留天数等不会改变卷来源的字段；用户可以在该状态显式发起“迁移到平台 PVC”。

迁移只能选择当前 VictoriaMetrics 节点：平台先校验节点就绪和 hostPath 存在性，缩容 Deployment 至零并等待 Pod 退出，自动创建目标 PVC 以及节点约束的迁移 Job。迁移 Job 在同一节点挂载源 hostPath 和目标 PVC，以 `tar` 复制数据并生成文件清单/校验结果；校验成功才更新 Deployment 为 PVC volume、恢复副本并等待 readiness。失败或新 Pod 未就绪时，平台恢复原有 hostPath Deployment 并保留目标 PVC 供诊断，绝不删除源目录。成功后源目录同样保留，清理由后续独立、明确确认的系统功能处理。

同节点转换避免目标 local-path 卷尚未绑定时跨节点复制和远程宿主机路径授权的复杂性。需迁移到其他节点时，先完成本次 hostPath-to-PVC 转换，再使用已有的本地 PVC 跨节点迁移流程。

### Alertmanager 保持专用受控生命周期

Alertmanager 继续使用 `cylism-alertmanager-data`，默认 `1Gi`，挂载 `/alertmanager`。Alertmanager 与 VictoriaMetrics PVC 的名称、容量、StorageClass 和生命周期均由所属组件 API 自动控制；存储页只展示只读状态和跳转，不提供创建、编辑、扩容、备份、导入、迁移、删除或清理操作。告警卸载仍保留 PVC 与通知 Secret，后续重新启用会重新挂载同一 PVC。

## Risks / Trade-offs

- [local-path PVC 绑定到错误节点] → 在创建前校验目标节点就绪，Deployment 从首次创建起携带 nodeSelector，并返回实际绑定节点。
- [hostPath 迁移复制失败或目标 Pod 不可用] → 切换前完整停止源工作负载，迁移 Job 输出校验结果；任何失败均恢复 hostPath Deployment，保留源目录和目标 PVC 诊断。
- [已有 hostPath 安装被误认为 PVC 安装] → 状态 API 返回显式存储模式，配置界面按模式限制可编辑字段，只有显式迁移操作可以替换 volume。
- [基础设施卷被误删或错误修改] → API 返回只读操作能力，前端隐藏操作；后端所有通用 PVC 写路径均拒绝基础设施标签。
- [PVC 容量不足] → 安装时要求正数容量和有效单位，状态页显示 request/capacity；扩容流程留待支持 StorageClass `allowVolumeExpansion` 时单独实现。
- [升级时 RBAC 缺失] → 部署前验证服务账号在 `monitoring` 命名空间具有 create/get/update PVC 权限，并在 API 返回明确权限错误。

## Migration Plan

1. 部署包含 PVC 分类和 VictoriaMetrics PVC 安装能力的平台版本，并确认 `monitoring` 的 PVC RBAC 已应用。
2. 新安装 VictoriaMetrics 时由平台创建 `cylism-victoria-metrics-data`，部署携带 nodeSelector 并等待 PVC/工作负载就绪。
3. 已有 hostPath 安装升级后保持原 Deployment volume；管理员可发起同节点迁移，平台暂停写入、复制并校验数据，切换成功后保留源目录。
4. 迁移失败或切换后新 Pod 未就绪时，平台恢复 hostPath Deployment；目标 PVC 留存供诊断，不删除任何源数据。
5. 回滚时保留两个基础设施 PVC。回滚到旧版本仅会使 VictoriaMetrics PVC 实例不再由旧 UI 配置，不会删除数据；恢复新版本后可重新识别。

## Open Questions

- PVC 扩容是否在首期开放，取决于集群 StorageClass 的 `allowVolumeExpansion` 支持情况。
