## Design

The backend owns three fixed VictoriaMetrics instant queries. Each query calculates only positive `delta` values over one of the existing monitoring windows (`1h`, `6h`, `24h`, `7d`) and returns at most twelve rows:

- `node_filesystem_avail_bytes` for node mount-point growth.
- `kubelet_volume_stats_used_bytes` for PVC growth.
- `container_fs_usage_bytes` for approximate container writable-filesystem growth.

An optional node filter is validated and escaped before being added as a PromQL label matcher. The API does not accept browser-provided PromQL. PVC rows are enriched from the Kubernetes Pod list by matching `persistentVolumeClaim.claimName` in the same namespace.

The frontend adds a lazy Monitoring Disk tab. It fetches the new endpoint only when selected or refreshed, and shows three dense ranked tables. It deliberately does not infer individual directories; host directory analysis needs an explicit, later, on-demand privileged diagnostic design.

## Bounds

- Only existing `1h`, `6h`, `24h` and `7d` monitoring ranges are allowed.
- Each fixed query returns at most twelve rows.
- Only positive growth is displayed.
- The endpoint shares the existing VictoriaMetrics readiness checks and query timeout behavior.
