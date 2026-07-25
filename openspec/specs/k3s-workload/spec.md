# k3s-workload Specification

## Purpose
TBD - created by archiving change unified-api-response. Update Purpose after archive.
## Requirements
### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

### Requirement: Deployment 列表查看
系统 SHALL 提供 Deployment 列表 API，返回所有命名空间的 Deployment 信息。

#### Scenario: 获取 Deployment 列表
- **WHEN** GET /api/k8s/deployments
- **THEN** 返回 Deployment 列表，每项含 name, namespace, replicas, ready, images, age

### Requirement: Deployment 详情查看
系统 SHALL 提供 Deployment 详情 API，包含关联 Pod 列表和 ReplicaSet 历史。

#### Scenario: 获取 Deployment 详情
- **WHEN** GET /api/k8s/deployments/:namespace/:name
- **THEN** 返回 Deployment 完整 spec/status 及关联 Pod 列表

#### Scenario: 获取 Deployment 关联 Pod
- **WHEN** GET /api/k8s/deployments/:namespace/:name/pods
- **THEN** 返回该 Deployment 管理的所有 Pod，含 name, status, node, ip, restarts

#### Scenario: 获取 Deployment 版本历史
- **WHEN** GET /api/k8s/deployments/:namespace/:name/revisions
- **THEN** 返回 ReplicaSet 历史列表，含 revision, image, replicas, created_at

### Requirement: Deployment 扩缩容
系统 SHALL 支持通过 PATCH 修改 Deployment 副本数。

#### Scenario: 扩容 Deployment
- **WHEN** PATCH /api/k8s/deployments/:namespace/:name/scale body {replicas: 5}
- **THEN** Deployment spec.replicas 更新为 5，返回成功

#### Scenario: 缩容至零
- **WHEN** PATCH /api/k8s/deployments/:namespace/:name/scale body {replicas: 0}
- **THEN** Deployment spec.replicas 更新为 0，所有 Pod 终止

### Requirement: Deployment 镜像更新
系统 SHALL 支持通过 PATCH 更新 Deployment 容器镜像。

#### Scenario: 更新容器镜像
- **WHEN** PATCH /api/k8s/deployments/:namespace/:name/image body {container: "app", image: "nginx:1.25"}
- **THEN** 指定容器的镜像更新，触发滚动更新

### Requirement: Deployment 回滚
系统 SHALL 支持 Deployment 回滚到指定版本。

#### Scenario: 回滚到上一版本
- **WHEN** POST /api/k8s/deployments/:namespace/:name/rollback body {revision: 0}
- **THEN** Deployment 回滚到上一个 revision，返回成功

### Requirement: StatefulSet 列表与详情
系统 SHALL 提供 StatefulSet 列表和详情 API。

#### Scenario: 获取 StatefulSet 列表
- **WHEN** GET /api/k8s/statefulsets
- **THEN** 返回 StatefulSet 列表，含 name, namespace, replicas, ready, images

#### Scenario: 获取 StatefulSet 详情
- **WHEN** GET /api/k8s/statefulsets/:namespace/:name
- **THEN** 返回 StatefulSet 完整信息及关联 Pod 列表

### Requirement: StatefulSet 扩缩容
系统 SHALL 支持 StatefulSet 扩缩容。

#### Scenario: 扩容 StatefulSet
- **WHEN** PATCH /api/k8s/statefulsets/:namespace/:name/scale body {replicas: 3}
- **THEN** StatefulSet spec.replicas 更新为 3，按序创建新 Pod

### Requirement: DaemonSet 列表与详情
系统 SHALL 提供 DaemonSet 列表和详情 API。

#### Scenario: 获取 DaemonSet 列表
- **WHEN** GET /api/k8s/daemonsets
- **THEN** 返回 DaemonSet 列表，含 name, namespace, desired, ready, node_selector

#### Scenario: 获取 DaemonSet 详情
- **WHEN** GET /api/k8s/daemonsets/:namespace/:name
- **THEN** 返回 DaemonSet 完整信息及关联 Pod 列表

### Requirement: 前端工作负载页面
前端 SHALL 提供工作负载管理页面，以 Tab 切换 Deployment/StatefulSet/DaemonSet，支持列表查看及变更操作。

#### Scenario: 页面 Tab 切换
- **WHEN** 用户点击 StatefulSet Tab
- **THEN** 展示 StatefulSet 列表，隐藏 Deployment 和 DaemonSet 列表

#### Scenario: 扩缩容操作
- **WHEN** 用户在 Deployment 列表点击扩缩容按钮并输入新副本数
- **THEN** 发送 PATCH 请求，成功后刷新列表显示新副本数

#### Scenario: 更新镜像操作
- **WHEN** 用户在 Deployment 操作菜单中选择更新镜像并输入新镜像 tag
- **THEN** 发送 PATCH 请求，成功后提示"镜像更新已触发滚动更新"

#### Scenario: 回滚操作
- **WHEN** 用户在 Deployment 操作菜单中选择回滚并确认版本
- **THEN** 发送 POST rollback 请求，成功后提示回滚完成

