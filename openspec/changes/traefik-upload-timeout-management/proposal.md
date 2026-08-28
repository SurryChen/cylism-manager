# Traefik Upload Timeout Management

## Why

平台托管的 OCI 制品库通过 Traefik Ingress 接收 Docker Registry blob 上传。Traefik 的默认入口请求读取超时为 60 秒；在网络吞吐较低或大镜像层上传时，网关会中断尚未完成的 PUT 请求，客户端得到 499，Registry 仅能记录客户端提前断开。

当前系统组件页面虽已通过 HelmChartConfig 管理 Traefik 的部分 values，但其通用表单不会解析或保留 `additionalArguments`。手工修改 HelmChartConfig 会脱离平台的期望状态、审计与恢复能力，并可能在下一次页面保存时被覆盖。

## What Changes

- 在“基础设施 -> 系统组件 -> Traefik -> 配置”中增加受管的入口请求读取超时配置。
- 将同一个受管超时同时写入 `web` 和 `websecure` Traefik entrypoint 的 `readTimeout` 参数。
- 显示未覆盖时的 Traefik 默认值（60 秒）；管理员显式保存后才创建或更新该项配置。
- 保存后观察 HelmChartConfig 调谐结果和 Traefik Deployment 实际启动参数，明确显示已生效、未生效或失败。
- 恢复默认时删除平台托管的 HelmChartConfig 与期望配置记录，使 K3s 恢复 bundled Traefik 默认行为。

## Non-Goals

- 不管理 Traefik 的任意静态参数或第三方 Helm chart values。
- 不修改 OCI Registry 的 Deployment、PVC、认证 Secret 或 Ingress 资源。
- 不处理 Traefik 之外的外部 CDN、反向代理、Tailscale 或网络链路性能问题。
- 不将超时设为无限制；平台只接受明确的正时长并提供受控预设。

## Capabilities

### Modified Capabilities

- `system-components`: 对 Helm 管理的 Traefik 增加入口读取超时的结构化编辑、持久化、实际参数校验和恢复默认行为。

## Impact

- `internal/api`：解析、校验并渲染 Traefik 专属的受管 Helm values，返回实际生效状态。
- `internal/k8s`：读取 Traefik Deployment 参数以验证两个 entrypoint 的有效超时。
- `web/src/views/SystemComponents.vue`：为 Traefik 增加结构化超时控件与应用状态展示。
- `internal/api/*_test.go`、`internal/k8s/*_test.go`、`web/src/views/*.test.js`：覆盖配置渲染、验证、状态检测和 UI 交互。
