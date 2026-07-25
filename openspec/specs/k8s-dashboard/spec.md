# k8s-dashboard Specification

## Purpose
TBD - created by archiving change k3s-cluster-control. Update Purpose after archive.
## Requirements
### Requirement: 集群摘要 API
Dashboard API SHALL 提供 K3s 集群关键指标总览，包括节点数、Pod 数、Deployment 数、Service 数、命名空间数、K3s 版本。

#### Scenario: 获取集群摘要
GIVEN K8s 客户端可用
WHEN GET /api/k8s/dashboard
THEN 返回 nodes_total, pods_total, pods_ready, deployments_total, deployments_ready, services_total, namespaces, version

#### Scenario: 获取 Pod 列表
GIVEN K8s 客户端可用
WHEN GET /api/k8s/pods?namespace=default
THEN 返回该命名空间下 Pod 列表，含 name, namespace, status, node, ip, restarts

#### Scenario: 获取 Service 列表
GIVEN K8s 客户端可用
WHEN GET /api/k8s/services
THEN 返回 Service 列表，含 name, namespace, cluster_ip, type, ports

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: Dashboard 前端展示增强
前端 Dashboard 页面的集群概况 Strip SHALL 新增 Deployment 总数/就绪数和 Service 总数展示。

#### Scenario: 展示 Deployment 统计
- **WHEN** Dashboard 页加载且 K8s 可用
- **THEN** 集群概况 Strip 展示 "Deployments: N/M Ready" 统计

#### Scenario: 展示 Service 统计
- **WHEN** Dashboard 页加载且 K8s 可用
- **THEN** 集群概况 Strip 展示 "Services: N" 统计

