## Context

平台已支持在 Environment 中创建 `ReadWriteOnce` PVC、从 PV node affinity 读取绑定节点，并为带可写 PVC 的 Deployment 使用单副本和 `Recreate` 策略。K3s 默认 local-path Provisioner 将卷目录落在首次消费者节点；该目录不能被另一个节点的 Pod 同时挂载。

因此，迁移不能是零停机操作。平台需要把必要的停止窗口、数据复制、节点调度切换与清理操作组织成一个可审计、可恢复的异步任务，而不能直接修改既有 PV 的 node affinity 或让用户手工执行节点命令。

## Goals / Non-Goals

**Goals:**

- 提供数值加单位的 PVC 容量编辑，支持 `Mi`、`Gi`、`Ti`，默认 `Gi`。
- 支持迁移一个已 Bound、平台托管、K3s local-path/hostPath、RWO PVC 到一个已就绪的目标节点。
- 自动更新受影响应用的模板与当前工作负载，并用内部 Release 记录切换后的运行态。
- 在服务短暂停止期间复制完整数据，失败时恢复源工作负载；成功后让用户确认再清理源卷。
- 将任务、阶段、诊断、字节进度和可清理状态持久化，以便平台重启后继续查询并处理未完成任务。

**Non-Goals:**

- 不提供零停机、本地卷双写、增量同步、跨集群迁移、备份或快照。
- 不迁移未由 Cylism 管理、非 Bound、非 local-path/hostPath、RWX/ROX PVC，也不改变 StorageClass。
- 不修改 Kubernetes 既有 PV 的 source 或 node affinity，不以 `kubectl`/shell 文本伪造 PV 重绑定。
- 不自动永久删除源数据；删除源 PVC 必须独立确认并遵从其 PV reclaim policy。

## Decisions

### 使用替换 PVC 而不是原地修改 PV

迁移创建带新名称的目标 PVC，由临时挂载 Pod 触发 `WaitForFirstConsumer` Provisioner 在目标节点生成 PV 和目录。数据复制后，平台将关联模板中的 claim 名称替换为目标 PVC，并通过内部迁移 Release 渲染目标 Deployment。源 PVC、PV 和 Release 快照不被篡改。

这避免依赖 PV 绑定、claimRef、node affinity 的可变性，也让失败时源卷仍能恢复。代价是目标 Claim 名称会带迁移后缀，前端必须清楚显示“迁移自”的关系。

备选方案是删除源 PVC 后以原名称重建或直接修改 PV node affinity。前者在 `Delete` reclaim policy 下可能先删除数据，后者依赖 Provisioner 实现和 Kubernetes 不保证的可变字段，均不采用。

### 以临时绑定 Pod 预配目标 local-path 卷

迁移在停止源应用之前创建目标 PVC 和带 `kubernetes.io/hostname=<target>` 的临时 Pod。该 Pod 使用可配置的 pause 镜像，只挂载目标 PVC，等待其 Bound 后读取目标 PV 的 hostPath/local 路径。预配失败时源应用不受影响。

平台将新增一个可配置的迁移辅助镜像，默认 `registry.k8s.io/pause:3.10`。用户网络不能访问该镜像时可以配置已验证的私有镜像。备选方案是直接创建静态 PV；这会绕开 StorageClass、扩大 PV 写权限和长期维护面，不采用。

### 停止后一次性 SSH 流式复制

目标卷预配完成后，平台将关联 Deployment 缩容至零并等待引用源 PVC 的 Pod 终止。随后通过两个受控 SSH 进程将 `sudo tar` 的 stdout 从源节点直接管道到目标节点的 stdin，使用 `--numeric-owner` 保留属主和权限，数据不会写入平台容器磁盘。

迁移仅支持 Kubernetes Node 能映射到平台 Server、该 Server 采用 SSH 密钥认证、且 `sudo -n`、`tar`、目标磁盘空间检查均通过的场景。密码认证和平台无法通过 SSH 管理的节点会在预检阶段拒绝。一次全量复制使停机长度与数据量成正比；后续增量同步作为独立能力评估。

### 独立迁移记录与通用步骤日志

新增 `PersistentVolumeMigration` 表保存 Environment、源/目标 PVC、源/目标节点、关联 Application、当前状态、复制字节数、源发布/模板快照、错误与时间戳。通用 `OperationLog` 使用 `resource_type=pvc_migration` 记录阶段详情。迁移状态包括：`pending`、`preflight`、`provisioning_target`、`stopping_source`、`copying`、`cutover`、`waiting_ready`、`succeeded`、`failed`、`rolling_back`、`rolled_back`、`cleanup_pending`、`cleaned`。

启动时和查询时收敛未完成任务：尚未停止源应用的任务可标为失败并清理临时资源；已停止但未切换的任务优先恢复源 Deployment；已切换任务通过目标 Deployment Ready 状态判断成功或发起回滚。

### 受限关联与切换

首期只允许源 PVC 被一个平台 Application 的一个运行中 Deployment 引用；未受控工作负载、多个应用或多节点引用均阻断迁移。该 Application 下引用源 PVC 的模板会同步改为目标 PVC 和目标节点，避免下一次发布回到源卷。迁移创建新的内部 Release，Release 快照在切换前复制并仅替换卷名和节点；历史 Release 保持不可变，并因源卷保留而仍可审计。

任务开始后在 PVC 与 Application 上建立锁。创建发布、保存相关模板、删除 PVC 和重复迁移都必须返回冲突，直到任务终态。

### 成功后的清理和回滚

目标 Deployment Ready 后，任务进入 `cleanup_pending`，源 PVC/PV 保留。用户明确确认“清理源卷”后，平台再次确认目标 Deployment Ready、源卷没有引用，再删除源 PVC；底层数据处理遵从显示的 reclaim policy。

在切换前或目标工作负载未就绪时，平台删除临时目标 Pod/PVC，恢复模板快照与源 Deployment 副本数/节点约束。复制完成后也不删除源数据，确保回滚存在有效来源。

## Risks / Trade-offs

- [本地 RWO 不能双挂载] → UI 明确展示短暂停机，先预配目标卷再停止源应用，缩小受影响时间。
- [源节点故障或 SSH 中断] → 复制前置完整预检；复制失败不清理源卷，尝试恢复源 Deployment 并记录脱敏错误。
- [目标目录空间不足或辅助镜像不可拉取] → 在停止源应用前检查空间并等待临时 Pod Bound；失败直接终止任务。
- [数据复制中的权限、特殊文件或内容变化] → 复制在 Pod 已停止后执行，使用 sudo tar 的数字 uid/gid 语义；不支持在线数据库一致性或增量同步。
- [新旧 PVC 并存造成误操作] → 新 PVC 显示迁移来源，源 PVC 标记迁移锁，清理需要二次确认。
- [平台重启] → 状态和步骤持久化，启动后的收敛逻辑不会在未知状态自动删除源数据。
- [SSH 参数或路径注入] → PVC/PV/Server 数据均重新读取并校验；所有 SSH 参数使用 argv，远程路径通过严格 shell 转义，不记录密钥或命令中的敏感值。

## Migration Plan

1. 将 GORM 模型自动迁移部署到 SQLite，并更新 `k8s/platform-deployment.yaml` 的 Pod 与 PVC 权限。
2. 发布新平台版本后，现有 PVC 和模板不发生任何变化；仅新建迁移任务使用替换 PVC。
3. 任务失败或取消时，按持久化阶段执行幂等清理与源 Deployment 恢复。
4. 成功任务保留源卷，管理员验证应用数据后从任务详情显式确认清理。

## Open Questions

- 首期是否将迁移辅助镜像放入系统设置，还是固定使用默认 pause 镜像。设计选择可配置默认值，以覆盖无公网环境。
- 首期只支持一个运行中 Deployment 引用；多个应用共享同一 PVC 的协调迁移留给后续能力。
