## MODIFIED Requirements

### Requirement: 节点列表
系统 SHALL 展示集群中所有节点的名称、IP、状态、角色、K8s 版本、资源使用和派生的节点健康状态。

#### Scenario: 展示故障节点
- **WHEN** Node 的 Ready Condition 为 `Unknown`，且最后心跳时间距当前超过五分钟
- **THEN** 系统返回 `health_state=failed`、最后心跳时间和故障原因，前端将节点标记为故障

#### Scenario: 短暂未就绪不视为故障
- **WHEN** Node 的 Ready Condition 为 `False`，或最后心跳时间距当前不超过五分钟
- **THEN** 系统返回 `health_state=not_ready`，且不提供强制驱逐入口

### Requirement: 节点驱逐
系统 SHALL 支持安全驱逐节点上的工作负载，并保留每个未完成驱逐的真实错误原因。

#### Scenario: PDB 阻止普通驱逐
- **WHEN** 普通驱逐的 Eviction API 返回 PDB 拒绝
- **THEN** 系统保留对应 Pod、命名空间和 Kubernetes 原始错误，并在前端结果中展示

#### Scenario: 非 PDB 的 Eviction 429
- **WHEN** 普通驱逐的 Eviction API 返回 HTTP 429 但错误不表示 PDB 拒绝
- **THEN** 系统将其标记为提交驱逐失败，不得误称为 PDB 保护

## ADDED Requirements

### Requirement: 故障节点强制驱逐
系统 SHALL 仅允许对已判定故障的 worker 节点执行经风险确认的强制驱逐。

#### Scenario: 成功强制驱逐受控 Pod
- **WHEN** 用户对 `health_state=failed` 的 worker 提交 `acknowledge_risk=true` 且 `confirm_node_name` 精确匹配节点名
- **THEN** 系统 cordon 节点，并直接删除受控制器管理且非 DaemonSet/static Pod，绕过 PDB，并返回逐 Pod 结果

#### Scenario: 拒绝非故障节点
- **WHEN** 用户对 `ready` 或 `not_ready` 节点请求强制驱逐
- **THEN** 系统拒绝请求且不删除任何 Pod

#### Scenario: 拒绝 control-plane
- **WHEN** 用户对 control-plane 节点请求强制驱逐
- **THEN** 系统拒绝请求且返回控制面节点不支持强制驱逐的错误

#### Scenario: 拒绝缺失确认
- **WHEN** 风险确认未勾选或确认节点名不匹配
- **THEN** 系统拒绝请求且不修改 Node 或 Pod

#### Scenario: 保留无控制器 Pod
- **WHEN** 故障节点上存在无控制器管理的 Pod
- **THEN** 系统不删除该 Pod，在结果中标记为 blocked，并说明该 Pod 不会自动重建

#### Scenario: 跳过 DaemonSet 与 static Pod
- **WHEN** 故障节点上存在 DaemonSet 或 static/mirror Pod
- **THEN** 系统不删除该 Pod，在结果中标记为 skipped

### Requirement: 服务器与集群节点绑定
系统 SHALL 仅在 Kubernetes Node 仍存在时保留服务器的集群角色与节点名绑定。

#### Scenario: 平台移出节点后解绑服务器
- **WHEN** 平台成功移除一个 Kubernetes Node
- **THEN** 系统清空所有绑定该节点的服务器的 `cluster_role` 和 `k8s_node_name`

#### Scenario: 集群外部移出节点后自愈解绑
- **WHEN** 查询服务器列表时成功读取到 Kubernetes Node 列表，且某服务器绑定的 Node 已不存在
- **THEN** 系统清空该服务器的 `cluster_role` 和 `k8s_node_name` 并持久化更新

#### Scenario: 集群不可用时保留绑定
- **WHEN** 查询服务器列表时无法读取 Kubernetes Node 列表
- **THEN** 系统不修改服务器的现有集群绑定

#### Scenario: 手动解绑服务器
- **WHEN** 用户确认解除一个服务器的集群绑定
- **THEN** 系统仅清空该服务器的 `cluster_role` 和 `k8s_node_name`，不得删除 Kubernetes Node 或影响 Pod

### Requirement: 交互式终端可靠性
系统 SHALL 为 SSH 和 Pod 终端提供可靠的大文本粘贴、文本选择与可读的浅色终端画布。

#### Scenario: 粘贴长命令
- **WHEN** 用户向 SSH 或 Pod 终端粘贴不超过 1 MiB 的文本
- **THEN** 系统完整转发文本，不因 WebSocket 分片或部分读取关闭会话

#### Scenario: 选择终端输出
- **WHEN** 用户跨方向拖选终端输出以复制
- **THEN** 终端不因遮罩点击而关闭，只能通过明确的关闭操作结束

#### Scenario: 浅色终端画布
- **WHEN** 用户打开 SSH 或 Pod 终端
- **THEN** 终端以不透明白色背景、深色文本和可见选区展示，不随平台夜间配色变为灰色透底
