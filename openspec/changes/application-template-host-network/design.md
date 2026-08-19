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

host-network Pod 的监听端口直接占用节点端口。对于单节点或指定节点上的固定端口应用，默认滚动更新会在旧 Pod 退出前创建新 Pod，并因端口冲突使新 Pod 无法调度。因此，Deployment 在 `host_network=true` 时使用 `Recreate` 策略；该策略会先停止旧 Pod，再创建替换 Pod。

未启用 `host_network` 的 Deployment 继续使用 Kubernetes 默认滚动更新行为。StatefulSet 的更新策略不在此次变更范围内。

### 3. 已解决事件不影响当前运行状态

发布详情的运行状态仅将 Kubernetes Warning Event 作为未就绪 Pod 的当前诊断。已处于 `Running` 且所有容器已 `Ready` 的 Pod 可能保留历史调度或启动 Warning；这些已解决事件不得将当前运行状态标记为异常。

### 4. UI 使用高级网络开关

模板编辑器在既有“服务网络”区域增加一个默认关闭的复选框。保存和编辑时该值随模板 Spec 往返；不推导 hostPort，也不修改 Service 类型或端口。

## Risks

- host-network Pod 与节点进程或同节点其他 Pod 的监听端口可能冲突，Kubernetes 调度器会拒绝冲突 Pod。本次不新增预检。
- host-network Deployment 在发布期间会短暂不可用，因为 `Recreate` 必须先终止旧 Pod 才能释放节点端口。本次不新增端口冲突预检。
