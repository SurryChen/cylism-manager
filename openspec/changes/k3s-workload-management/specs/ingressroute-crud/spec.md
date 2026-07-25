## MODIFIED Requirements

### Requirement: IngressRoute 完整 CRUD
Traefik IngressRoute 管理 SHALL 支持列表、创建、更新、删除，对接真实 K8s dynamic client。路由页 SHALL 检测并标注当前使用的 Ingress Controller 类型。

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

#### Scenario: Traefik Controller 检测标记
- **WHEN** 路由页加载
- **THEN** 检测 ingressroutes.traefik.io CRD 及 Traefik Deployment 状态
- **AND** 在页面顶部 Banner 展示控制器信息

#### Scenario: 双 Tab 切换
- **WHEN** 用户点击标准 Ingress Tab
- **THEN** 展示标准 K8s Ingress 列表并与 IngressRoute 列表可自由切换
