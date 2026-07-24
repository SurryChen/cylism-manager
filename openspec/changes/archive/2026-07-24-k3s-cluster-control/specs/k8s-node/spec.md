## ADDED Requirements

### Requirement: 节点列表对接真实 K8s
Node API SHALL 从 K8s Node API 获取实时节点信息，支持驱逐和移除操作。

#### Scenario: 获取集群节点
GIVEN K8s 客户端可用
WHEN GET /api/nodes
THEN 从 K8s Node API 获取节点列表，返回 name, status, role, version, ip

#### Scenario: 驱逐节点
GIVEN K8s 客户端可用
WHEN POST /api/nodes/:id/drain
THEN cordon 节点并驱逐非 DaemonSet Pod

#### Scenario: 移除节点
GIVEN K8s 客户端可用
WHEN DELETE /api/nodes/:id
THEN 从集群删除 Node 资源
