## Why

当前平台的 local-path `ReadWriteOnce` PVC 在首次挂载后固定于单个节点。应用需要迁移节点或节点维护时，用户只能停服务后手动复制目录、重建卷和修改模板，过程容易造成数据丢失或调度错误。

## What Changes

- 将 PVC 容量编辑改为数值输入和单位选择，前端继续向现有 API 提交 Kubernetes Quantity。
- 新增平台托管 local-path PVC 的迁移任务：预检节点与 SSH 权限、预配目标 PVC、受控停止应用、流式复制数据、切换应用模板和工作负载、等待就绪。
- 持久化迁移任务和步骤日志，支持页面轮询、失败诊断、自动回滚及成功后的显式源卷清理。
- 在迁移期间锁定源 PVC 及关联应用的发布、模板修改和删除操作，防止并发改变数据路径。
- 更新平台 RBAC，使其可以管理仅用于迁移的临时 Pod 和受管 PVC 生命周期。

## Capabilities

### New Capabilities
- `pvc-local-volume-migration`: 受控迁移平台托管的本地 RWO PVC 与其关联应用。

### Modified Capabilities
- `persistent-storage`: 容量输入使用数值与单位控件，并在受管本地 PVC 上提供迁移任务入口和状态。

## Impact

- 影响 `internal/k8s` 的 PVC/PV、临时 Pod、Deployment 等 Kubernetes 操作，`internal/api` 的迁移 API 与 SSH 流式复制，及 GORM/SQLite 的迁移任务持久化。
- 影响 `PersistentVolumes.vue`、应用模板校验与发布流程；历史 Release 快照保持不可变。
- 需要源、目标节点均已作为使用 SSH 密钥认证且可免密 sudo 的平台服务器绑定到 Kubernetes Node。迁移不新增第三方依赖。
