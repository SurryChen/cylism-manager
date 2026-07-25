# k3s-service-discovery Specification

## Purpose
TBD - created by archiving change unified-api-response. Update Purpose after archive.
## Requirements
### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: Service 列表查看
系统 SHALL 提供 Service 列表 API（已存在，扩展 Endpoint 数量字段）。

#### Scenario: 获取 Service 列表
- **WHEN** GET /api/k8s/services
- **THEN** 返回 Service 列表，每项含 name, namespace, type, cluster_ip, ports, endpoint_count

### Requirement: Service 详情查看
系统 SHALL 提供 Service 详情 API，包含端口映射和 Selector 标签。

#### Scenario: 获取 Service 详情
- **WHEN** GET /api/k8s/services/:namespace/:name
- **THEN** 返回 Service 完整信息，含 name, namespace, cluster_ip, type, ports[], selector{}

### Requirement: EndpointSlice 查询
系统 SHALL 提供按 Service 查询关联 EndpointSlice 及端点的 API，优先使用 EndpointSlice，不可用时 fallback 到传统 Endpoints。

#### Scenario: 通过 EndpointSlice 获取端点
- **WHEN** GET /api/k8s/services/:namespace/:name/endpoints
- **AND** discovery.k8s.io/v1 EndpointSlice API 可用
- **THEN** 返回 EndpointSlice 列表，每个 Slice 含 name, address_type, endpoints[] (ip, node, pod_name, ready)

#### Scenario: Fallback 到传统 Endpoints
- **WHEN** GET /api/k8s/services/:namespace/:name/endpoints
- **AND** EndpointSlice API 不可用
- **THEN** 从 v1/Endpoints 查询并返回等价的端点数据结构

### Requirement: 前端服务发现页面
前端 SHALL 提供服务发现页面，展示 Service 列表，点击展开显示 Selector→EndpointSlice→Endpoint 映射链路。

#### Scenario: Service 列表展示
- **WHEN** 用户打开服务发现页面
- **THEN** 展示 Service 列表，含名称、类型、ClusterIP、端口、端点数量

#### Scenario: 展开 Service 端点详情
- **WHEN** 用户点击某个 Service 行
- **THEN** 展开面板显示 Selector 标签、EndpointSlice 列表及每个端点的 IP/端口/状态/所属 Pod

#### Scenario: 端点状态直观展示
- **WHEN** EndpointSlice 包含 NotReady 端点
- **THEN** 该端点以警告色（var(--warning)）标识，帮助排查连通性问题

