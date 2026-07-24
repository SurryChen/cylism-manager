# ingressroute-crud Specification

## Purpose
TBD - created by archiving change k3s-cluster-control. Update Purpose after archive.
## Requirements
### Requirement: IngressRoute 完整 CRUD
Traefik IngressRoute 管理 SHALL 支持列表、创建、更新、删除，对接真实 K8s dynamic client。

#### Scenario: 创建 IngressRoute
GIVEN K8s 客户端可用
WHEN POST /api/routes body {name, namespace, host, service_name, service_port}
THEN 创建 Traefik IngressRoute CRD 资源并返回成功

#### Scenario: 列出 IngressRoute
GIVEN K8s 客户端可用
WHEN GET /api/routes
THEN 返回所有 IngressRoute，含 name, namespace, host, service, tls 字段

#### Scenario: 删除 IngressRoute
GIVEN IngressRoute 已存在
WHEN DELETE /api/routes/:namespace/:name
THEN 删除 IngressRoute CRD 资源

