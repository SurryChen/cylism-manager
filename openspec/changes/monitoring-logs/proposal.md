## Why

平台当前可查看指标和告警，但无法集中检索应用容器日志；故障排查仍要手工定位命名空间、Pod 并执行 `kubectl logs`。需要在现有监控工作区内提供由平台管理、可按应用和运行对象过滤的日志采集与检索能力。

## What Changes

- 在 `monitoring` 命名空间托管单实例 Loki 与每节点 Grafana Alloy DaemonSet，采集 Kubernetes 容器的 stdout/stderr 日志。
- 为 Loki 自动创建并管理基础设施 PVC，支持选择数据节点、容量、StorageClass 与日志保留天数；通用存储页面只读展示该系统卷。
- 提供经平台后端代理的日志状态、配置、标签选项和范围查询 API；Loki 不暴露到 Ingress，浏览器不能直接访问其查询接口。
- 在集群监控页面新增“日志”分栏，支持按时间范围、项目、环境、应用、命名空间、Pod、容器、节点和关键字过滤，并展示可定位的日志行。

## Capabilities

### New Capabilities

- `monitoring-logs`: 托管容器标准输出日志采集、保留和受限检索。

### Modified Capabilities

- `infrastructure-storage`: 将 Loki 数据 PVC 作为平台基础设施存储纳入清单与通用操作保护。
- `monitoring`: 将日志检索加入既有监控工作区。

## Impact

- 修改 `internal/k8s` 创建、协调和查询 Loki/Alloy、Service、ConfigMap、RBAC 与基础设施 PVC。
- 修改 `internal/api` 注册日志状态、安装、配置、筛选与查询接口，并实施查询边界。
- 修改 `web/src/views/Monitoring.vue` 及相关组件，新增日志工作区。
- 新增镜像依赖 `grafana/loki` 与 `grafana/alloy`；节点必须允许 DaemonSet 只读挂载 `/var/log/pods` 与 `/var/log/containers`。
