## Why

VictoriaMetrics 仍把时序数据直接写入用户填写的节点目录，Alertmanager 的 PVC 虽已创建却未在存储页明确归属。监控和告警数据需要纳入统一的基础设施存储管理，避免宿主机路径分散、难以盘点和误操作。

## What Changes

- VictoriaMetrics 的新安装改为创建并挂载平台托管 PVC，使用所选节点的 `nodeSelector` 保证 local-path 卷与工作负载一致。
- Alertmanager 与 VictoriaMetrics PVC 在存储页标识为基础设施资源，显示归属、使用方与容量状态。
- 存储页提供基础设施筛选和跳转到对应监控设置的入口；基础设施 PVC 仅允许其所属组件自动创建和管理，禁止通用用户修改操作。
- 已有基于 `hostPath` 的 VictoriaMetrics 安装保持可用并提供显式迁移到系统管理 PVC 的流程，包含停机、复制校验、切换和可回滚的旧目录保留。

## Capabilities

### New Capabilities

- `infrastructure-storage`: 管理监控和告警组件的托管 PVC、基础设施存储库存与受控操作边界。

### Modified Capabilities

- None.

## Impact

- 修改 `internal/k8s` 的 VictoriaMetrics PVC/Deployment 构造、状态识别和基础设施 PVC 分类。
- 修改监控安装与配置 API、PVC 列表 API 及对应测试。
- 修改监控与存储 Vue 页面、表单和组件测试。
- 平台服务账号需要继续拥有 `monitoring` 命名空间的 PVC 管理权限。
