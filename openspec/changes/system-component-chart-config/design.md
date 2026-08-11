# Design: System Component Chart Config

## Persistence

平台不依据组件名称或 K3s 版本直接选择控制机制，而是每次读取、保存与重放前检查实际控制源。K3s 版本只作为诊断信息，不能覆盖当前集群资源的事实。

| 模式 | 探测条件 | 写入方式 |
| --- | --- | --- |
| `helm_chart` | 存在同 namespace、同名 HelmChart | HelmChartConfig |
| `static_deployment` | 无 HelmChart，存在同名 Deployment | 平台受控 Deployment 字段 |
| `embedded` | ServiceLB 检测到 `svclb-*` DaemonSet 且无同名 Deployment | 只读 |
| `unknown` | 没有足以判断的运行时证据 | 拒绝写入 |

`helm_chart` 模式操作 `helm.cattle.io/v1` HelmChartConfig CRD（dynamic client）。K3s helm-controller 每次 reconcile（含升级）都按 CRD 重新渲染内置 chart，因此配置持久。

`static_deployment` 模式保存 `valuesContent` 作为期望记录，并仅直接更新 `replicas`、`strategy.rollingUpdate` 与 Pod 模板的 `nodeSelector.kubernetes.io/hostname`。选择节点会改变 Pod 模板，从而由 Deployment 控制器按滚动策略在目标节点创建新 Pod；清空选择器则恢复 Kubernetes 调度。Manager 启动后及每五分钟重放已保存的静态组件配置，处理 K3s 静态清单的重新应用。CoreDNS 是该模式的已确认实例。

## API

```text
GET  /api/system-components
     → [{ chart_name, namespace, controller_mode, detection_evidence,
          capabilities, values_content, apply_status, apply_error,
          last_applied_at, deployment: { replicas, ready_replicas,
          available_replicas, strategy, image, fixed_node } }]
PUT  /api/system-components/:chart        body: { values_content }
POST /api/system-components/:chart/revert
```

白名单：

```go
var systemChartWhitelist = map[string]string{
    "coredns": "kube-system",
    "traefik": "kube-system",
    "metrics-server": "kube-system",
    "local-path-provisioner": "kube-system",
    "servicelb": "kube-system",
}
```

## Safety

- chart 名白名单，拒绝未知组件。
- valuesContent 用 sigs.k8s.io/yaml 校验合法后才会应用。CoreDNS 还要求副本至少为 1、RollingUpdate 同时包含 `maxUnavailable` 和 `maxSurge`，且目标节点必须处于 Ready、可调度状态。
- 写操作复用 JWT 与 AuditMiddleware。
- Helm 组件恢复默认 = 删除 CRD + 删除记录。静态组件恢复平台受控字段的通用 Kubernetes 默认值并移除 hostname selector，再删除记录；CoreDNS 保持其 K3s 清单的已知默认副本与滚动策略。内置和未知组件不允许写入或恢复。

## Console

`/cluster?tab=system-components`：表格展示组件状态。CoreDNS 显示当前固定节点，提供“迁移”入口和节点选择器；“安全滚动基线”生成 `replicas: 2` 与 `deploymentStrategy.rollingUpdate.maxUnavailable: 0 / maxSurge: 1`。恢复默认需确认。
