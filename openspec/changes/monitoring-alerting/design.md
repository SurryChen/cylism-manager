## Context

现有平台在 `monitoring` 命名空间运行单副本 VictoriaMetrics 和 node-exporter，并通过平台 API 查询指标。它没有规则评估、通知去重或静默能力。集群规模为少量节点和几十个 Pod，目标是优先发现单节点故障、资源饱和与 Kubernetes 工作负载失效，而非建设高可用监控系统。

## Goals / Non-Goals

**Goals:**

- 使用 `vmalert` 每分钟从现有 VictoriaMetrics 评估平台托管规则，并将事件发往 Alertmanager。
- 按需运行最小化 kube-state-metrics，以提供 Pod 状态和 Deployment/StatefulSet 副本规则所需指标。
- 由单副本 Alertmanager 完成分组、去重、重试、静默和告警状态保存。
- 在监控页以活跃告警为主视图，提供静默、恢复记录、告警设置与关联资源跳转。
- 首期可靠地投递飞书机器人消息，且不向浏览器或数据库暴露 Webhook URL 明文。
- 让 Alertmanager 可选择与 VictoriaMetrics 不同的就绪节点，并为其状态数据申请 1Gi `ReadWriteOnce` PVC。

**Non-Goals:**

- 不提供 Alertmanager 或 vmalert 的多副本 HA、跨集群告警、外部黑盒探测或 S3/OSS 备份。
- 不实现任意 PromQL 规则编辑器；首期仅调整预置规则的启用状态、阈值和持续时间。
- 不承诺整个集群或外网同时不可达时仍能通知；该场景需要独立于集群的外部探测。
- 不将全部时序指标、完整事件正文或 Secret 明文写入 SQLite。

## Decisions

### 托管 vmalert 与 Alertmanager

在 `monitoring` 命名空间创建 `cylism-vmalert`、`cylism-alertmanager` 和 `cylism-kube-state-metrics`。vmalert 通过 ClusterIP Service 查询 VictoriaMetrics，并以一分钟评估间隔向 Alertmanager 提交事件。kube-state-metrics 仅读取 nodes、pods、deployments、statefulsets 和 daemonsets，并由 VictoriaMetrics 只在告警启用期间采集。Alertmanager 使用 1 个副本、1Gi PVC 和指定节点的 `kubernetes.io/hostname` NodeSelector；vmalert 与 kube-state-metrics 不需要持久卷。

选择单副本是因为当前规模下告警组件的预估 request 为 vmalert `100m/128Mi`、Alertmanager `100m/128Mi`、kube-state-metrics `50m/64Mi`。与直接由平台定时查询并发送通知相比，Alertmanager 原生提供分组、去重、静默和失败重试；与完整 kube-prometheus-stack 相比，避免引入 Prometheus、Grafana 和额外 CRD 的资源消耗与运维面。

### 规则与配置的持久化边界

平台将可公开的告警配置、渲染后的 vmalert 规则和 Alertmanager 路由写入受标签管理的 ConfigMap；飞书 Webhook URL 与随机回调令牌写入 Kubernetes Secret。平台 API 仅读取 Secret 是否已配置，不返回明文。Alertmanager 的 `nflog`、静默和通知状态存入 PVC。

选择 Kubernetes 资源而非 SQLite 是因为配置必须在平台 Pod 重启或版本回退后仍供告警组件直接使用，且避免额外的数据迁移。活跃告警从 Alertmanager 读取；平台通知中继在内存中保留最多 12 条已成功转发的恢复事件，因此重启后恢复列表会清空。长期审计历史不在本变更范围内。

### 飞书通知中继

Alertmanager 的 generic webhook 载荷不兼容飞书机器人格式。因此 Alertmanager 将批量事件提交至平台内部 Service 的 `/api/monitoring/alerts/notify`。该路由不经过 JWT，但必须携带 Secret 中保存的 bearer token；平台用常量时间比较校验后，将事件渲染为飞书交互卡片并投递到 Secret 中的 Webhook URL。

此方案比新增独立转换镜像更少消耗资源并可复用平台审计、错误处理和 HTTP 客户端。中继地址仅使用集群内 Service DNS，不暴露到 Ingress。通知失败将向 Alertmanager 返回非 2xx，使其按自身退避策略重试。

### 规则与告警工作区

监控页新增 `告警` 同级视图。顶部是活跃、今日触发、静默和通知通道健康摘要；主体按严重度显示活跃告警，随后显示有限的最近恢复记录。节点和工作负载告警提供跳转到对应现有监控视图的操作。静默操作仅提供固定时长和说明，设置抽屉管理飞书渠道与预置规则。

不在页面中引入第二层 Tab，以免操作路径分散；规则、渠道和静默记录属于低频配置，收纳在右侧抽屉中。

## Risks / Trade-offs

- [Alertmanager 所在节点故障] → 首次安装默认优先选择与 VictoriaMetrics 不同的就绪节点，并在页面展示部署节点；单副本仍无法覆盖该节点自身失效。
- [整个集群或平台服务不可用] → 明确标注集群内告警边界；部署外部探测属于后续独立能力。
- [飞书 Webhook 泄露] → URL 仅写入 Secret，API 响应脱敏；内部回调要求随机 bearer token 并使用常量时间校验。
- [规则误报或通知风暴] → 预置 `for` 持续时间、Alertmanager 分组/重复间隔、页面静默与测试通知均为默认能力。
- [local-path PVC 与节点绑定] → Alertmanager 在 PVC 首次绑定前即带有 NodeSelector；安装状态显示卷与 Pod 的实际节点，失败不删除 PVC 数据。

## Migration Plan

1. 部署包含告警 API 和 Kubernetes 资源构造的新平台版本，并确保其 RBAC 可管理 `monitoring` 命名空间的 ConfigMap、Secret、Deployment、Service 和 PVC。
2. 在监控页选择告警节点并配置飞书 Webhook，平台创建 Secret、PVC、配置和工作负载。
3. 等待 vmalert 与 Alertmanager 就绪，发送测试通知后启用默认规则。
4. 发生回滚时删除 vmalert、Alertmanager、Service 和 ConfigMap，但保留 Alertmanager PVC 与 Secret，避免丢失静默状态和通知渠道；恢复到新版本可重新接管这些资源。

## Open Questions

- 首期只接入飞书机器人；企业微信、Telegram 和邮件作为后续可插拔渠道。
- 首期恢复记录由平台内存中的有界通知队列提供，长期历史和确认人审计后续再引入数据库模型。
