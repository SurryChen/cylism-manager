## ADDED Requirements

### Requirement: Lightweight Service inventory query

The system SHALL support `GET /api/k8s/services?endpoint_count=false` as a lightweight Service inventory query. It SHALL return the existing Service metadata fields required by the resource list and SHALL omit per-Service EndpointSlice or Endpoints lookups and the `endpoint_count` response field.

#### Scenario: Load a lightweight Service inventory

- **WHEN** a client requests `GET /api/k8s/services?endpoint_count=false`
- **THEN** the system SHALL issue one Service list operation for the requested namespace scope
- **AND THEN** the system SHALL NOT query EndpointSlice or Endpoints for each returned Service

#### Scenario: Preserve the detailed Service list contract

- **WHEN** a client requests `GET /api/k8s/services` without `endpoint_count=false`
- **THEN** the system SHALL retain the existing response behavior including `endpoint_count`

## MODIFIED Requirements

### Requirement: 前端服务发现页面
前端 SHALL 提供服务发现页面，展示轻量 Service 列表，并在用户请求详情时展示 Selector→EndpointSlice→Endpoint 映射链路。

#### Scenario: Service 列表展示
- **WHEN** 用户打开服务发现页面
- **THEN** 前端 SHALL 使用轻量 Service inventory 查询展示名称、命名空间、类型、ClusterIP、端口和年龄
- **AND THEN** 列表 SHALL NOT 展示端点数量

#### Scenario: 展开 Service 端点详情
- **WHEN** 用户点击某个 Service 行的“查看端点”操作
- **THEN** 前端 SHALL 查询该 Service 的端点详情并显示 Selector 标签、EndpointSlice 列表及每个端点的 IP/端口/状态/所属 Pod

#### Scenario: 端点状态直观展示
- **WHEN** EndpointSlice 包含 NotReady 端点
- **THEN** 该端点 SHALL 以警告色（var(--warning)）标识，帮助排查连通性问题
