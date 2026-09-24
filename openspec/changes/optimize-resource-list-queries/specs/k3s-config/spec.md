## MODIFIED Requirements

### Requirement: ConfigMap 列表查看
系统 SHALL 提供 ConfigMap 列表 API。轻量查询 `GET /api/k8s/configmaps?usage=false` SHALL 返回名称、命名空间、键数量和年龄，且 SHALL NOT 扫描或返回关联工作负载信息；不带该参数的既有查询 SHALL 保持原有引用信息行为。

#### Scenario: 获取轻量 ConfigMap 列表
- **WHEN** GET /api/k8s/configmaps?usage=false
- **THEN** 系统 SHALL 返回 ConfigMap 列表，每项含 name、namespace、keys_count 和 age
- **AND THEN** 系统 SHALL NOT 查询 Deployment、StatefulSet 或 DaemonSet 来填充 used_by

#### Scenario: 获取完整 ConfigMap 列表
- **WHEN** GET /api/k8s/configmaps 未传入 usage=false
- **THEN** 系统 SHALL 保持返回关联工作负载信息的既有行为

### Requirement: Secret 列表查看
系统 SHALL 提供 Secret 列表 API，不返回敏感值。轻量查询 `GET /api/k8s/secrets?usage=false` SHALL 返回名称、命名空间、类型、键元数据和年龄，且 SHALL NOT 扫描或返回关联工作负载信息；不带该参数的既有查询 SHALL 保持原有引用信息行为。

#### Scenario: 获取轻量 Secret 列表
- **WHEN** GET /api/k8s/secrets?usage=false
- **THEN** 系统 SHALL 返回不含 Secret 值和 used_by 的列表数据
- **AND THEN** 系统 SHALL NOT 查询 Deployment、StatefulSet 或 DaemonSet 来填充引用关系

#### Scenario: 获取完整 Secret 列表
- **WHEN** GET /api/k8s/secrets 未传入 usage=false
- **THEN** 系统 SHALL 保持返回关联工作负载信息的既有行为

### Requirement: 前端配置管理页面
前端 SHALL 提供 ConfigMap/Secret 配置管理页面，以轻量列表展示资源，并按需读取资源详情和引用关系。

#### Scenario: 展示轻量配置列表
- **WHEN** 用户打开 ConfigMap 或 Secret Tab
- **THEN** 前端 SHALL 请求对应的 usage=false 列表
- **AND THEN** 列表 SHALL 展示名称、命名空间、类型（仅 Secret）、键数量和年龄
- **AND THEN** 列表 SHALL NOT 展示引用数量

#### Scenario: 查看配置引用关系
- **WHEN** 用户点击 ConfigMap 或 Secret 的“查看”操作
- **THEN** 前端 SHALL 请求该资源详情
- **AND THEN** 前端 SHALL 在详情中展示关联工作负载或其为空状态

#### Scenario: 删除被引用的配置资源
- **WHEN** 用户请求删除被工作负载或平台记录引用的 ConfigMap 或 Opaque Secret
- **THEN** 后端 SHALL 拒绝删除并返回冲突原因
