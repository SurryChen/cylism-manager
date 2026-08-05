## Context

现有平台已在 `monitoring` 命名空间托管 VictoriaMetrics、Alertmanager、vmalert、kube-state-metrics 和 node-exporter。平台通过后端访问集群内服务，监控页面已使用顶层分栏呈现概览、工作负载与告警。基础设施 PVC 已有统一标签、存储页库存展示与通用写操作保护。

应用日志目前仅存在于每个节点的 Kubernetes 容器日志文件中，需要先定位 Pod 才能通过 `kubectl logs` 查看。目标集群规模较小，首期优先以单实例、短保留、受限查询的方式缩短排障路径，不建设日志高可用或跨集群聚合。

## Goals / Non-Goals

**Goals:**

- 收集全部 Kubernetes 容器的 stdout/stderr，保留 Kubernetes 元数据并可按平台项目、环境、应用过滤。
- 使用平台托管的 Loki 持久化日志，并在通用存储页可见但禁止普通 PVC 操作。
- 通过平台 API 查询日志，限制时间范围、条数、查询长度和超时，避免任意 LogQL 与高成本查询影响集群。
- 在监控工作区提供易用的时间范围、范围筛选、关键字检索和日志上下文展示。
- 允许安装后调整保留天数，清晰显示采集和存储组件是否就绪。

**Non-Goals:**

- 不采集容器内任意文件、宿主机 syslog、审计日志或应用 APM trace。
- 不提供实时 tail、保存查询、日志告警、复杂 LogQL 编辑器、跨集群查询或 Loki 多副本 HA。
- 不将原始日志写入 SQLite，也不向公网通过 Ingress 暴露 Loki。
- 不对旧节点历史日志进行回填；安装前已经轮转或删除的日志不可恢复。

## Decisions

### 使用 Loki + Grafana Alloy

在 `monitoring` 命名空间运行一个 `cylism-loki` StatefulSet，使用单个 `ReadWriteOnce` 平台基础设施 PVC 挂载到 `/loki`。Loki 采用本地 filesystem object store、单体模式和可配置的 retention，只有 ClusterIP Service 对集群内开放。

每个可调度节点运行 `cylism-alloy` DaemonSet。Alloy 以只读方式挂载 `/var/log/pods`、`/var/log/containers` 和 `/var/lib/kubelet/pods`，通过 Kubernetes API 发现 Pod 元数据，读取 CRI 格式日志后推送至 Loki Service。选择 Alloy 而非 Promtail，因为 Promtail 已结束生命周期，并且 Alloy 是 Grafana 推荐的后续采集器。

首期资源请求/限制为 Loki `100m/256Mi` 与 `500m/512Mi`，Alloy 每节点 `25m/64Mi` 与 `100m/128Mi`。默认 Loki PVC 为 `10Gi`、保留 `14` 天。Loki 与 VictoriaMetrics 一样在 PVC 首次消费前绑定管理员选定的就绪节点，避免 local-path PVC 落在不期望的节点。

### 日志标签与平台筛选

Alloy 为每条日志附加受控标签：`namespace`、`pod`、`container`、`node`、`workload`、`project`、`environment` 和 `release`（存在时）。应用发布资源必须继续带有平台当前的管理标签，以便 Alloy 从 Pod 标签映射为日志标签；后端再将数据库中的项目、环境和应用 ID 校验并映射为对应命名空间和工作负载筛选。

不将 Pod UID、请求 ID、用户 ID、完整镜像地址或自由文本字段设为标签，避免 Loki 索引基数无界增长。关键字使用受平台生成的行过滤表达式，而不是标签。

API 的下拉选项由 Kubernetes 当前对象和平台数据库生成，不依赖扫描 Loki 全量标签；这使筛选控件可快速加载，也不会因为日志过期而丢失当前应用的可选项。

### 受限日志查询 API

浏览器调用平台的受 JWT 保护接口；后端根据项目、环境、应用等结构化筛选构造 LogQL selector，可选关键字以转义后的行过滤条件附加。接口不接受原始 LogQL。

每次请求必须指定或使用默认时间范围，最大范围为 24 小时，最大返回 500 行，最大关键字长度为 256 字符，后端查询超时为 10 秒。接口返回时间戳、行文本和标签摘要，并带回已使用的范围与是否命中上限。所有筛选 ID 均按现有项目/环境关系校验，防止跨工作区枚举日志。

出现 Loki 不可用、采集器未就绪或超出查询边界时，API 返回可操作错误，不能伪造为空结果。

### 生命周期、配置与存储边界

日志安装仅在 Kubernetes 客户端可用、所选节点已就绪时执行。平台创建或协调 Loki StatefulSet、ClusterIP Service、数据 PVC、Alloy ServiceAccount/ClusterRole/Binding、ConfigMap 以及 Alloy DaemonSet。状态接口同时报告 Loki、Alloy 期望与就绪数量、存储节点、PVC 名称、容量、StorageClass 与 retention。

修改 retention 只更新 Loki 配置并触发滚动更新；已有日志将由 Loki 后台清理。数据节点和 PVC 规格在已有数据后不允许直接修改，避免 local-path PVC 迁移或数据破坏；迁移能力属于后续明确变更。卸载删除工作负载、Service、ConfigMap 和权限资源，但保留 Loki PVC 数据，重新安装时可接管该 PVC。

Loki PVC 使用现有基础设施标签，并新增 `InfrastructureLoki = "loki"` 归属。存储库存将其展示为“Loki 日志存储”，并让普通创建、扩容、删除、导入、备份、迁移与恢复接口拒绝对其操作，提示从监控日志设置管理。

### 监控页面体验

在 `/#/monitoring` 增加“日志”顶层分栏。未安装时显示安装表单（数据节点、容量、StorageClass、保留天数）；安装中或异常时显示组件状态与重新检测入口。就绪后页面顶部使用紧凑摘要展示 Loki 状态、Alloy 就绪节点数、日志保留期和存储卷。

日志查询区按“时间范围 + 多选下拉筛选 + 关键字 + 查询按钮”组织。项目、环境和应用使用层级筛选，后续筛选按当前范围收窄；每项可清除。结果按时间倒序展示，单行包括时间、命名空间/Pod/容器、日志正文和展开后的标签。查询仅在用户点击按钮或切换明确时间范围后执行，页面首次进入不会自动执行全量日志查询。设置收纳在居中高层级弹窗，复用告警设置的遮罩与层级策略。

## Risks / Trade-offs

- [日志包含敏感信息] → 首期仅允许已登录平台用户通过后端查询；不写入 SQLite、不通过 Ingress 暴露 Loki，并在页面提示应用不要输出凭据。细粒度 RBAC/脱敏策略后续单独设计。
- [Loki 数据盘不足] → 默认短保留和容量状态展示；Loki 强制 retention。真实容量依应用日志量变化，安装表单明确容量不是硬配额，仍需监控 PVC 使用率。
- [Alloy 可读取节点日志] → 仅授予读取 Pod 元数据所需的 Kubernetes RBAC，宿主机日志目录全为只读挂载，不挂载 Docker/containerd socket。
- [标签高基数] → 平台固定标签集合，关键字一律作为内容过滤，拒绝用户提交自由 LogQL。
- [旧节点与日志路径差异] → 状态和 DaemonSet 事件应展示采集器失败原因；首期面向 K3s/containerd 标准 CRI 路径，其他运行时兼容性需实测。

## Migration Plan

1. 部署包含日志采集 API、Kubernetes 资源构造和监控页面的新平台版本，确保服务账号可管理 `monitoring` 命名空间的 StatefulSet、DaemonSet、Service、ConfigMap、PVC、ServiceAccount、ClusterRole 与 ClusterRoleBinding。
2. 管理员在监控日志分栏选择就绪节点、存储规格和 retention，平台创建托管资源。
3. 等待 Loki 就绪与 Alloy 在目标节点启动，查询近一小时少量日志验证元数据标签和关键字过滤。
4. 回滚时删除新版本创建的计算与配置资源，保留 Loki PVC；恢复到新版本可再次接管数据。若需要彻底擦除日志，由后续受控的基础设施日志卸载/数据清理操作完成。

## Open Questions

- 首期将应用、项目和环境作为日志筛选字段；按租户授权隔离日志需要与平台的权限模型演进同步设计。
- 实时 tail 和从发布详情/POD 状态一键携带筛选跳转在采集与检索稳定后再增加。
