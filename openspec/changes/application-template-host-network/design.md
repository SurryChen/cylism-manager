## Context

应用模板已经以 `ReleaseSpec` JSON 保存，并作为不可变发布快照的一部分渲染 Kubernetes 工作负载。该模型已承载 Service、端口和节点选择等 Pod 级配置，因此 `host_network` 应成为模板的显式布尔字段，而不是允许用户注入任意 Kubernetes YAML。

## Decisions

### 1. 使用带默认值的布尔字段

`ReleaseSpec` 增加：

```json
{"host_network": true}
```

缺失字段按 `false` 处理，保障已有模板和历史发布快照兼容。

### 2. 渲染完整的 Kubernetes 语义

当字段为 `true` 时，渲染的 PodSpec 同时设置：

```yaml
hostNetwork: true
dnsPolicy: ClusterFirstWithHostNet
```

DNS 策略必须随 host networking 设置，否则 Pod 会失去预期的集群 DNS 解析行为。

本变更保留现有工作负载更新策略。Deployment 的 `RollingUpdate`/`Recreate` 行为不在此次变更范围内。

### 3. UI 使用高级网络开关

模板编辑器在既有“服务网络”区域增加一个默认关闭的复选框。保存和编辑时该值随模板 Spec 往返；不推导 hostPort，也不修改 Service 类型或端口。

## Risks

- host-network Pod 与节点进程或同节点其他 Pod 的监听端口可能冲突，Kubernetes 调度器会拒绝冲突 Pod。本次不新增预检。
- 使用默认滚动更新的单节点固定端口工作负载可能暂时 Pending。本次按范围不改变策略，后续可独立处理发布策略和端口冲突预检。
