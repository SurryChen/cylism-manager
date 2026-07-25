# k3s-ingress-std Specification

## Purpose
TBD - created by archiving change unified-api-response. Update Purpose after archive.
## Requirements
### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: 标准 Ingress 列表查看
系统 SHALL 提供标准 K8s Ingress 列表 API。

#### Scenario: 获取 Ingress 列表
- **WHEN** GET /api/k8s/ingresses
- **THEN** 返回 Ingress 列表，每项含 name, namespace, hosts[], paths[], tls[], controller, age

### Requirement: 标准 Ingress 详情查看
系统 SHALL 提供标准 Ingress 详情 API。

#### Scenario: 获取 Ingress 详情
- **WHEN** GET /api/k8s/ingresses/:namespace/:name
- **THEN** 返回 Ingress 完整信息，含 rules (host, paths -> service_name:service_port), tls, annotations

### Requirement: 标准 Ingress 创建
系统 SHALL 支持创建标准 K8s Ingress 资源。

#### Scenario: 创建 Ingress
- **WHEN** POST /api/k8s/ingresses body {namespace, name, host, paths[{path, service_name, service_port}]}
- **THEN** 创建 Ingress 资源并返回成功

### Requirement: 标准 Ingress 删除
系统 SHALL 支持删除标准 K8s Ingress 资源。

#### Scenario: 删除 Ingress
- **WHEN** DELETE /api/k8s/ingresses/:namespace/:name
- **THEN** 删除 Ingress 资源并返回成功

### Requirement: Ingress Controller 状态检测
系统 SHALL 检测当前集群中运行的 Ingress Controller 类型和状态。

#### Scenario: Traefik Controller 检测
- **WHEN** GET /api/system/crds 返回 ingressroutes.traefik.io 存在
- **AND** Traefik Deployment 在 kube-system 命名空间中 Running
- **THEN** 路由页面顶部 Banner 显示"Inrgess Controller: Traefik vX.X（运行中）"

#### Scenario: 无已知 Controller
- **WHEN** 未检测到 Traefik 或其他已知 Ingress Controller
- **THEN** 路由页面顶部 Banner 显示"未检测到 Ingress Controller，Ingress 规则可能无法生效"

### Requirement: 前端路由页双 Tab
前端路由页 SHALL 提供 IngressRoute 和 Ingress 双 Tab 切换。

#### Scenario: Tab 切换至标准 Ingress
- **WHEN** 用户点击"标准 Ingress" Tab
- **THEN** 展示标准 K8s Ingress 列表，隐藏 IngressRoute 列表

#### Scenario: Traefik 状态 Banner 展示
- **WHEN** 路由页加载且 Traefik Controller 已检测
- **THEN** 页面顶部 Banner 展示控制器名称和版本，标注"当前使用 Traefik"

