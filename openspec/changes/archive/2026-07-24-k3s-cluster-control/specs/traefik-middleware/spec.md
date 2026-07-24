## ADDED Requirements

### Requirement: Middleware 列表展示
平台 SHALL 支持列出 Traefik Middleware CRD 资源，了解集群中已配置的中间件规则。

#### Scenario: 列出 Middleware
GIVEN K8s 客户端可用
WHEN GET /api/routes/middlewares
THEN 返回所有 Traefik Middleware，含 name, namespace, type

### Requirement: TLS Store 列表展示
平台 SHALL 支持列出 TLS Store 和 TLS Option 资源。

#### Scenario: 列出 TLS Store
GIVEN K8s 客户端可用
WHEN GET /api/routes/tls-stores
THEN 返回所有 TLS Store/TLS Option 资源
