# persistent-storage Specification

## Purpose
TBD - created by archiving change pvc-management-and-application-mounts. Update Purpose after archive.
## Requirements
### Requirement: 环境级 PVC 管理
系统 SHALL 允许用户在指定 Environment 的 Namespace 中创建、查看和删除 Cylism 托管 PVC。首期创建的 PVC 必须使用 `ReadWriteOnce` 访问模式，并展示实际绑定节点与底层数据回收策略。

#### Scenario: 创建 PVC
- **WHEN** 用户为 Environment `production` 提交名称 `karakeep-data`、容量 `5Gi` 和有效 StorageClass
- **THEN** 系统在该 Environment Namespace 创建带 Cylism 管理标签和 Environment 标签的 PVC
- **AND** PVC 列表展示 Kubernetes 返回的 phase、容量和 StorageClass

#### Scenario: 查看本地 PVC 的绑定节点
- **WHEN** local-path PVC 已由 Pod 首次挂载并 Bound 到一个带 node affinity 的 PV
- **THEN** 系统展示该 PVC 的 Kubernetes Node Name 和对应的平台服务器自定义名称
- **AND** 系统展示该 PV 的 reclaimPolicy

#### Scenario: PVC 跨环境不可见
- **WHEN** 两个 Environment 使用不同 Namespace
- **THEN** 用户查询其中一个 Environment 的 PVC 列表
- **THEN** 系统不得返回另一个 Environment Namespace 中的 PVC

#### Scenario: 删除被引用的 PVC
- **WHEN** PVC 仍被应用 Deployment 或上线模板引用
- **THEN** 系统拒绝删除并返回引用该 PVC 的应用或模板
- **AND** 不删除 Kubernetes PVC 资源

#### Scenario: 删除可能清除数据的 PVC
- **WHEN** 用户删除绑定 PV 的 reclaimPolicy 为 `Delete` 的未引用 PVC
- **THEN** 系统在执行删除前展示底层数据可能被 Provisioner 清除的警告
- **AND** 仅在用户显式确认后删除 PVC

### Requirement: 模板 PVC 挂载
系统 SHALL 允许应用上线模板声明当前应用 Environment 中的托管 PVC 挂载，并在发布时渲染到 Deployment。

#### Scenario: 发布带数据卷的单副本应用
- **WHEN** 单副本应用模板引用已 Bound 到 `storage-node-a` 的 `karakeep-data`、选择 `storage-node-a` 并挂载至 `/data`
- **THEN** 系统创建的 Deployment 包含对应的 `PersistentVolumeClaim` volume 与 `/data` volumeMount
- **AND** Deployment 使用 Recreate 更新策略和 `kubernetes.io/hostname=storage-node-a` 的 nodeSelector
- **AND** Deployment 不设置 PodSpec nodeName

#### Scenario: 多副本应用引用可写 PVC
- **WHEN** 用户提交 `replicas=2` 且模板含有 `read_only=false` PVC 挂载
- **THEN** 系统阻断模板保存或发布并说明 RWO PVC 仅支持单副本
- **AND** 不更新 Deployment

#### Scenario: PVC 不属于当前环境
- **WHEN** 应用模板引用的 PVC 不在当前应用 Environment 的 Namespace 中，或 Environment 管理标签不匹配
- **THEN** 系统拒绝保存或发布该模板
- **AND** 不挂载该 PVC

#### Scenario: 本地卷挂载到不同节点
- **WHEN** 模板选择的部署节点与已 Bound 本地 PVC 的节点不一致
- **THEN** 系统阻断模板保存或发布并返回 PVC 的实际绑定节点
- **AND** 不更新 Deployment

#### Scenario: 首次挂载 Pending 本地卷
- **WHEN** 模板引用使用 WaitForFirstConsumer 的 Pending 本地 PVC 且选择部署节点
- **THEN** 系统创建带该节点约束的 Deployment，由首次 Pod 调度触发 PVC 绑定
- **AND** 系统继续等待工作负载就绪

#### Scenario: Pending 本地卷未选择节点
- **WHEN** 模板引用使用 WaitForFirstConsumer 的 Pending 本地 PVC 且未选择部署节点
- **THEN** 系统阻断发布并要求选择节点
- **AND** 不创建 Deployment

#### Scenario: 非 WaitForFirstConsumer PVC 尚未 Bound
- **WHEN** 用户发布引用 phase 为 Pending 且不是 WaitForFirstConsumer 的 PVC 的模板
- **THEN** 发布预检返回 PVC 未就绪的明确错误
- **AND** 不创建或更新 Deployment

### Requirement: 结构化 PVC 容量输入
系统 SHALL 在创建平台托管 PVC 时将容量数值与 Kubernetes 二进制单位分开输入，并向 Kubernetes 提交合法 Quantity。

#### Scenario: 创建 Gi 容量的 PVC
- **WHEN** 用户填写容量数值 `5` 并选择单位 `Gi`
- **THEN** 前端向 PVC 创建 API 提交存储容量 `5Gi`
- **AND** PVC 列表继续显示 Kubernetes 返回的容量 Quantity

#### Scenario: 拒绝无效容量数值
- **WHEN** 用户未填写正整数容量或选择不受支持的单位
- **THEN** 前端阻止提交并展示字段校验状态
- **AND** 不调用 PVC 创建 API

### Requirement: 本地 PVC 迁移入口和状态展示
系统 SHALL 在受管本地 PVC 列表中展示迁移入口、进行中状态、目标节点和成功后的源卷清理入口。

#### Scenario: 显示可迁移本地卷
- **WHEN** 受管 PVC 已 Bound 且具有本地 PV 节点亲和性
- **THEN** 列表展示当前绑定服务器和迁移操作
- **AND** 迁移对话框仅列出不同于源节点的可用目标节点

#### Scenario: 显示迁移中的卷
- **WHEN** PVC 存在未终态迁移任务
- **THEN** 列表展示当前迁移阶段和步骤详情入口
- **AND** 禁用该 PVC 的删除与重复迁移操作

