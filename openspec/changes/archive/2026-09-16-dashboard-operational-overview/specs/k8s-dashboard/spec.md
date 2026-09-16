## MODIFIED Requirements

### Requirement: 集群摘要 API

Dashboard API SHALL 提供 K3s 集群关键指标总览，包括节点数、Pod 数、Deployment 数、Service 数、命名空间数、K3s 版本，并准确表达就绪状态、节点版本不一致和单项查询失败。接口 SHALL 支持短时缓存和并发刷新合并；失败指标不得静默降级为零值。

#### Scenario: 获取集群摘要

- **WHEN** K8s 客户端可用且资源查询成功
- **THEN** `GET /api/k8s/dashboard` 返回 `nodes_total`、`pods_total`、`pods_ready`、`deployments_total`、`deployments_ready`、`services_total`、`namespaces` 和 `version`

#### Scenario: Pod 就绪统计

- **WHEN** Pod 的 Phase 为 Running 但 `PodReady` Condition 不为 True
- **THEN** 该 Pod 不计入 `pods_ready`

#### Scenario: 节点版本不一致

- **WHEN** 集群节点报告多个不同的 K3s/Kubernetes 版本
- **THEN** 摘要返回可识别的不一致状态，前端不得使用任意第一个节点版本冒充集群版本

#### Scenario: 单项资源查询失败

- **WHEN** 节点、Pod、Deployment、Service 或 Namespace 的任一查询失败
- **THEN** 响应保留其他成功指标，并通过错误字段或 `partial` 状态标识失败指标；失败指标不得以 `0` 表示

#### Scenario: 获取 Pod 列表

- **WHEN** K8s 客户端可用
- **THEN** `GET /api/k8s/pods?namespace=default` 返回该命名空间下 Pod 列表，含 `name`、`namespace`、`status`、`node`、`ip`、`restarts`

#### Scenario: 获取 Service 列表

- **WHEN** K8s 客户端可用
- **THEN** `GET /api/k8s/services` 返回 Service 列表，含 `name`、`namespace`、`cluster_ip`、`type`、`ports`

### Requirement: Dashboard 前端展示增强

前端 Dashboard 页面的集群概况 Strip SHALL 展示 Deployment 总数/就绪数、Service 总数、节点数、命名空间数、Pod 就绪数和版本，并对加载中、部分失败和版本不一致提供可理解的状态。

#### Scenario: 展示 Deployment 统计

- **WHEN** Dashboard 页加载且 K8s 摘要成功
- **THEN** 集群概况 Strip 展示“Deployments 就绪”统计，格式为 `就绪数/总数`

#### Scenario: 展示 Service 统计

- **WHEN** Dashboard 页加载且 K8s 摘要成功
- **THEN** 集群概况 Strip 展示“Services”统计和 Service 总数

#### Scenario: 摘要正在加载

- **WHEN** K8s 摘要请求尚未完成
- **THEN** 集群概况 Strip 保留标题并对各指标显示 `—` 或加载占位

#### Scenario: 摘要部分失败

- **WHEN** K8s 摘要只成功返回部分资源类型
- **THEN** 对应指标显示不可用状态，其他成功指标仍可见，页面不得显示虚假的 `0`

#### Scenario: 节点版本不一致

- **WHEN** 摘要报告节点版本不一致
- **THEN** 版本单元显示“多版本”或等价警示，不显示任意节点的单一版本作为集群版本
