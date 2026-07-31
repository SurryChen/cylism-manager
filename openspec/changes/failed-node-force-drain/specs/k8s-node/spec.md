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

