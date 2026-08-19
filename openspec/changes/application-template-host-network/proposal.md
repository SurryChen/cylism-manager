## Why

部分 UDP/QUIC 服务需要绕过 Pod CNI 网络，例如底层 CNI MTU 小于 QUIC 握手数据报时。当前应用发布模板无法声明 Kubernetes `hostNetwork`，用户只能在发布后手动修改 Deployment，后续平台发布又会覆盖该修改。

## What Changes

- 为应用发布模板新增可选 `host_network` 字段，默认 `false`。
- 模板编辑器在服务网络区域提供“使用宿主机网络”复选框。
- 发布资源渲染在字段启用时设置 Pod `hostNetwork: true`、兼容的 DNS 策略和 `Recreate` 更新策略。

## Non-goals

- 不增加 hostPort 配置、端口冲突预检或副本/节点限制。
- 不改变未启用该字段的模板、发布快照和现有 Service 行为。

## Impact

- `ReleaseSpec`、模板 API、发布快照和 Kubernetes Pod 渲染将携带新字段。
- 模板编辑器将保存和回显网络模式。
