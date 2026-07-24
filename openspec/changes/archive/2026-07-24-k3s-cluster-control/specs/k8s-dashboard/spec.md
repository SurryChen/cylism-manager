## ADDED Requirements

### Requirement: 集群摘要 API
Dashboard API SHALL 提供 K3s 集群关键指标总览，包括节点数、Pod 数、命名空间数、K3s 版本。

#### Scenario: 获取集群摘要
GIVEN K8s 客户端可用
WHEN GET /api/k8s/dashboard
THEN 返回 nodes_total, pods_total, pods_ready, namespaces, version

#### Scenario: 获取 Pod 列表
GIVEN K8s 客户端可用
WHEN GET /api/k8s/pods?namespace=default
THEN 返回该命名空间下 Pod 列表，含 name, namespace, status, node, ip, restarts

#### Scenario: 获取 Service 列表
GIVEN K8s 客户端可用
WHEN GET /api/k8s/services
THEN 返回 Service 列表，含 name, namespace, cluster_ip, type, ports
