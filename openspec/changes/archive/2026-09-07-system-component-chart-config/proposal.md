# System Component Chart Config

## Why

原实现除 CoreDNS 外一律假设 K3s 内置系统组件由 helm-controller 管理。实际集群可能因 K3s 版本、禁用内置组件、定制安装或升级遗留而不同；对静态 Deployment 写入 HelmChartConfig 会成功但不会影响实际工作负载，反之直接修改 Helm 管理的 Deployment 会被 helm-controller 覆盖。

平台需要以持久化方式管理这些内置 chart 的 values（通过 HelmChartConfig CRD），并暴露“期望 vs 实际”的可见性，避免同类事故。

## What Changes

- 新增 `SystemComponentConfig` 存储模型，记录每个内置组件的 valuesContent、控制模式与 apply 状态。
- 运行时检测控制源：匹配 HelmChart 时使用 HelmChartConfig；无 HelmChart 且存在同名 Deployment 时使用受控静态 Deployment；ServiceLB 等 K3s 进程内组件为只读；无法识别时拒绝写入。
- 新增 HelmChartConfig CRD 读写能力（创建/更新/删除），用于仍由 Helm 管理的系统组件。
- 静态 Deployment 组件使用受控配置：副本数、RollingUpdate 策略和 `kubernetes.io/hostname` 选择器；保存目标节点即触发滚动迁移。
- 静态 Deployment 的期望配置保存后在 Manager 启动及定期检查时重放，以覆盖 K3s 静态清单的重新应用。
- 新增系统组件 API：列表（期望 vs 实际 Deployment 状态）、保存配置、恢复默认。
- `/cluster` 新增“系统组件”tab：展示组件状态、编辑 YAML、一键应用安全滚动策略、恢复默认。
- 内置 chart 白名单，防止任意 CRD 名被操作。

## Non-Goals

- 不管理第三方 chart 的安装（仍由 Chart 仓库页负责）。
- 不做任意 Helm values 的 schema 校验（仅校验 YAML 合法性与白名单）。
- 不提供任意 Deployment 字段编辑；CoreDNS 仅允许平台维护的副本、滚动策略和节点选择器。

## Impact

- `internal/model`、`internal/store`：新模型与 CRUD。
- `internal/k8s`：HelmChartConfig CRD 读写。
- `internal/api`：handler 与路由。
- `web`：SystemComponents 视图与 ClusterHub tab。
