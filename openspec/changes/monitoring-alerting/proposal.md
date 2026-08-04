## Why

平台已经能采集节点和工作负载指标，但无法在用户离开监控页面后主动发现节点、容量或工作负载风险。需要在集群内以较低资源成本执行告警规则、收敛重复通知，并提供可直接处置的告警工作区。

## What Changes

- 在监控模块中托管 `vmalert`、Alertmanager 和按需启用的 kube-state-metrics，复用现有 VictoriaMetrics 与 `monitoring` 命名空间。
- 提供节点离线、CPU、内存、根磁盘、Pod 重启/Pending、工作负载副本不足和核心监控组件不可用的默认规则，并允许调整启用状态、阈值与持续时间。
- 将 Alertmanager 的告警分组、静默和活跃告警状态接入平台 API。
- 首期支持飞书机器人通知：通知 URL 和内部回调令牌仅保存在 Kubernetes Secret，平台内置受令牌保护的中继将 Alertmanager 事件转换为飞书消息。
- 在集群监控页面新增告警视图，优先呈现活跃告警、恢复记录、静默操作和跳转到节点或工作负载的处置入口；规则与通知渠道放入设置抽屉。

## Capabilities

### New Capabilities

- `monitoring-alerting`: 托管集群内告警组件、规则、通知渠道、静默和告警工作区。

### Modified Capabilities

- None.

## Impact

- 修改 `internal/k8s` 以创建 Alertmanager、vmalert、kube-state-metrics、PVC、ConfigMap 和 Secret。
- 修改 `internal/api` 以暴露告警配置、状态、静默、测试通知和受保护的内部回调 API。
- 修改监控 Vue 页面、路由测试及 API 测试。
- 需要镜像 `victoriametrics/vmalert` 和 `quay.io/prometheus/alertmanager`；平台自身服务账号必须继续具备管理 `monitoring` 命名空间中 Secret 的权限。
