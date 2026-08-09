# System Component Chart Config

## Why

K3s 内置系统组件（coredns、traefik、metrics-server、local-path-provisioner、servicelb）由 helm-controller 管理。直接 patch Deployment 的修改会在 K3s 升级或重新渲染时被还原；且单副本 + `maxUnavailable: 1` 的策略会在滚动更新时造成 DNS 等关键服务空窗（CoreDNS 事故）。

平台需要以持久化方式管理这些内置 chart 的 values（通过 HelmChartConfig CRD），并暴露“期望 vs 实际”的可见性，避免同类事故。

## What Changes

- 新增 `SystemComponentConfig` 存储模型，记录每个内置 chart 的 valuesContent 与 apply 状态。
- 新增 HelmChartConfig CRD 读写能力（创建/更新/删除），平台只写 CRD，不直接修改 Deployment。
- 新增系统组件 API：列表（期望 vs 实际 Deployment 状态）、保存配置、恢复默认。
- `/cluster` 新增“系统组件”tab：展示组件状态、编辑 YAML、一键应用安全滚动策略、恢复默认。
- 内置 chart 白名单，防止任意 CRD 名被操作。

## Non-Goals

- 不管理第三方 chart 的安装（仍由 Chart 仓库页负责）。
- 不做任意 Helm values 的 schema 校验（仅校验 YAML 合法性与白名单）。
- 不直接 patch Deployment/StatefulSet。

## Impact

- `internal/model`、`internal/store`：新模型与 CRUD。
- `internal/k8s`：HelmChartConfig CRD 读写。
- `internal/api`：handler 与路由。
- `web`：SystemComponents 视图与 ClusterHub tab。
