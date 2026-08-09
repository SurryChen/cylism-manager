# Design: System Component Chart Config

## Persistence

平台只操作 `helm.cattle.io/v1` HelmChartConfig CRD（dynamic client）。K3s helm-controller 每次 reconcile（含升级）都按 CRD 重新渲染内置 chart，因此配置持久。页面同时展示期望值（CRD/DB）与实际值（对应 Deployment 的 replicas/ready/strategy/image），差异可见。

## API

```text
GET  /api/system-components
     → [{ chart_name, namespace, values_content, apply_status, apply_error,
          last_applied_at, deployment: { replicas, ready_replicas,
          available_replicas, strategy, image } }]
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
- valuesContent 用 sigs.k8s.io/yaml 校验合法后才会写 CRD。
- 写操作复用 JWT 与 AuditMiddleware。
- 恢复默认 = 删除 CRD + 删除记录，不改 Deployment。

## Console

`/cluster?tab=system-components`：表格展示组件状态；“应用安全滚动策略”为 coredns 等生成 `maxUnavailable: 0 / maxSurge: 1` 基线；支持直接编辑 YAML；恢复默认需确认。
