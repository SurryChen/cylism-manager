# k3s-config Specification

## Purpose
TBD - created by archiving change unified-api-response. Update Purpose after archive.
## Requirements
### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: ConfigMap 列表查看
系统 SHALL 提供 ConfigMap 列表 API，返回名称、命名空间、键数量及关联工作负载信息。

#### Scenario: 获取 ConfigMap 列表
- **WHEN** GET /api/k8s/configmaps
- **THEN** 返回 ConfigMap 列表，每项含 name, namespace, keys_count, used_by[]

### Requirement: ConfigMap 详情查看
系统 SHALL 提供 ConfigMap 详情 API，返回所有键值对。

#### Scenario: 获取 ConfigMap 详情
- **WHEN** GET /api/k8s/configmaps/:namespace/:name
- **THEN** 返回 ConfigMap 全部键值对及关联工作负载列表

### Requirement: Secret 列表查看
系统 SHALL 提供 Secret 列表 API，不返回敏感值（values 字段排除），保留类型和键名。

#### Scenario: 获取 Secret 列表
- **WHEN** GET /api/k8s/secrets
- **THEN** 返回 Secret 列表，每项含 name, namespace, type, keys[], used_by[]
- **AND** 列表中不包含 data 字段的值

### Requirement: Secret 详情查看（遮蔽模式）
系统 SHALL 提供 Secret 详情 API，返回 value 时标记为遮蔽状态，前端默认不展示明文。

#### Scenario: 获取 Secret 详情（遮蔽）
- **WHEN** GET /api/k8s/secrets/:namespace/:name
- **THEN** 返回 Secret 完整信息，data 字段值以 base64 编码传输
- **AND** 前端默认展示"••••••"，用户点击眼睛图标后显示解码值

### Requirement: 关联工作负载追踪
系统 SHALL 查询 Deployment/StatefulSet/DaemonSet 中对 ConfigMap/Secret 的引用，返回被哪些工作负载使用。

#### Scenario: 查询 Secret 关联的工作负载
- **WHEN** 查询 Secret 详情
- **THEN** 返回 used_by 列表，每项含 workload_kind, workload_name, volume_or_env

### Requirement: 前端配置管理页面
前端 SHALL 提供配置管理页面，以 Tab 切换 ConfigMap/Secret，支持列表查看和详情展开。

#### Scenario: ConfigMap 列表展示
- **WHEN** 用户打开配置页面并选中 ConfigMap Tab
- **THEN** 展示 ConfigMap 列表，含名称、命名空间、键数量、关联工作负载

#### Scenario: Secret 详情遮蔽展示
- **WHEN** 用户点击某个 Secret 行展开详情
- **THEN** 键值对中 value 默认显示"••••••"
- **WHEN** 用户点击眼睛图标
- **THEN** 显示解码后的明文 value

