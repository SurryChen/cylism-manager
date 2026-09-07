## Why

应用发布始终创建 Deployment，带有 PVC 的数据服务也无法使用 StatefulSet，并且工作负载页面无法对比两类控制器的挂载。

## What Changes

- 应用使用既有 `workload_kind` 选择 Deployment 或 StatefulSet。
- 支持将已有、单副本且使用现有 PVC 的应用在两类控制器间安全切换。
- 在工作负载列表中显示两类控制器的 PVC 挂载。

## Non-goals

- 不实现 StatefulSet `volumeClaimTemplates` 的多副本数据分片迁移。
- 不自动将所有带 PVC 的应用转换为 StatefulSet。
