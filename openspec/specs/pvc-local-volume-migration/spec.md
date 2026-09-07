# pvc-local-volume-migration Specification

## Purpose
TBD - created by archiving change pvc-local-volume-migration. Update Purpose after archive.
## Requirements
### Requirement: 受管本地 PVC 的受控迁移
系统 SHALL 允许用户将一个已 Bound 的平台托管 local-path/hostPath `ReadWriteOnce` PVC 迁移至另一个已就绪节点，并持久化迁移状态和步骤日志。

#### Scenario: 创建有效迁移任务
- **WHEN** 用户选择已 Bound 的受管本地 PVC、唯一关联的应用和一个不同的已就绪目标节点
- **THEN** 系统创建异步迁移任务，记录源/目标节点、源 PVC、目标 PVC 名称和受影响应用
- **AND** 任务状态与步骤日志可通过 API 查询

#### Scenario: 拒绝不受支持的 PVC 或引用
- **WHEN** 用户迁移未托管、Pending、非本地、非 RWO PVC，或 PVC 被多个应用或未受控工作负载引用
- **THEN** 系统拒绝创建迁移任务并说明不满足的条件
- **AND** 不创建临时 Pod、目标 PVC 或修改任何工作负载

### Requirement: 迁移前置检查和目标卷预配
系统 SHALL 在停止源应用前验证源/目标节点，并以临时 Pod 在目标节点预配目标 PVC。

#### Scenario: 目标卷预配失败
- **WHEN** 目标节点未就绪、未绑定平台 Server、SSH 密钥认证或 sudo/tar/磁盘检查失败，或临时 Pod 未能使目标 PVC Bound
- **THEN** 系统将任务标记为失败并保存脱敏诊断信息
- **AND** 源应用及源 PVC 保持运行且不被修改

#### Scenario: 目标本地卷预配成功
- **WHEN** 临时 Pod 在目标节点运行且目标 PVC 已 Bound
- **THEN** 系统读取目标 PV 的本地路径和节点亲和性
- **AND** 系统仅在路径与选择的目标节点一致时进入停止源应用阶段

### Requirement: 一致性复制与工作负载切换
系统 SHALL 在源 Pod 完全停止后复制本地卷数据，并通过内部 Release 将关联应用切换至目标 PVC 和目标节点。

#### Scenario: 成功切换应用
- **WHEN** 源 Deployment 已缩容且所有引用源卷的 Pod 都已终止，且数据流复制完成
- **THEN** 系统更新关联模板中的 PVC 名称和节点选择，并创建内部迁移 Release
- **AND** 新 Deployment 使用目标 PVC 与 `kubernetes.io/hostname` 的目标节点选择器
- **AND** 系统等待目标工作负载就绪后将任务标记为 `cleanup_pending`

#### Scenario: 切换或就绪等待失败
- **WHEN** 数据复制、内部 Release 或目标工作负载就绪失败
- **THEN** 系统保留源 PVC 数据并执行源模板和源 Deployment 的恢复
- **AND** 系统将任务标记为 `failed` 或 `rolled_back` 并记录失败阶段

### Requirement: 源卷保护和显式清理
系统 SHALL 在目标应用就绪后保留源 PVC/PV，直到用户明确确认清理。

#### Scenario: 成功迁移后保留源卷
- **WHEN** 目标工作负载已就绪
- **THEN** 系统展示源 PVC、源节点和底层 reclaim policy，并提供清理源卷操作
- **AND** 系统不得在没有清理确认的情况下删除源 PVC 或源数据

#### Scenario: 确认清理源卷
- **WHEN** 用户确认清理一个处于 `cleanup_pending` 的迁移任务
- **THEN** 系统再次确认目标工作负载就绪且源 PVC 没有引用，再删除源 PVC
- **AND** 系统展示 reclaim policy 可能导致底层数据删除的风险并记录清理结果

