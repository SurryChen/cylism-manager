# Design: Failed Node Force Drain

## Context

Kubernetes 的工作负载迁移是通过删除旧 Pod 并由 Deployment、StatefulSet 等控制器在其他节点创建替代 Pod 实现的，不存在通用的运行中 Pod 实时迁移。普通驱逐通过 Eviction API 受 PDB 约束，适用于节点仍健康且计划维护的场景。机器宕机时，平台需要提供经过严格限制的故障转移路径。

## Decision: Fault Classification

`NodeInfo` 增加 `health_state`、`health_reason` 和 `last_heartbeat_at`，由 Ready Condition 派生：

| 状态 | 条件 | 可执行操作 |
| --- | --- | --- |
| `ready` | `Ready=True` | 普通驱逐 |
| `not_ready` | `Ready=False`，或心跳距今不超过 5 分钟 | 普通驱逐，禁止强制 |
| `failed` | `Ready=Unknown` 且最后心跳超过 5 分钟 | 普通驱逐和故障节点强制驱逐 |

五分钟是避免 K3s agent 短暂重启被误判的固定 V1 阈值。`Ready=False` 即使表明异常，也要求运维人员先等待或继续排障，不能作为绕过 PDB 的依据。

## Decision: Force Drain Boundary

新增 `POST /api/nodes/:name/force-drain`：

```json
{
  "acknowledge_risk": true,
  "confirm_node_name": "worker-a",
  "delete_empty_dir_data": true
}
```

服务端按以下顺序执行：

1. 读取 Node，拒绝 `control-plane`、非 `failed` 节点、未通过风险确认或确认节点名不匹配的请求。
2. Cordon 节点，防止它恢复后收到新调度。
3. 生成与普通驱逐相同的 Pod 分类清单。
4. 对受控制器管理、且不是 DaemonSet/static/mirror 的 Pod 调用 Delete API，`gracePeriodSeconds=0`，不调用 Eviction API，因此绕过 PDB。
5. DaemonSet/static/mirror Pod 标记为 skipped；无控制器 Pod 标记为 blocked，不删除；记录每个删除请求的成功或失败。

StatefulSet 仍属于可删除的受控 Pod，但结果文案提示其替代副本可能等待 PV detach/attach。平台不伪造“迁移完成”，只报告删除请求已提交。

## Decision: Result Model And Errors

扩展 `DrainResult`，增加 `forced` 标记与 `blocked`、`skipped` 结果列表，复用 `DrainPod`。普通驱逐的 pending 项保留 Kubernetes 原始错误文本；仅当文本确实表示 PDB 拒绝时才增加 PDB 前缀。

前端在普通驱逐完成后显示结果对话框，而不是关闭后只显示数量横幅。强制驱逐对话框展示受影响 Pod 和固定风险说明，只有状态为 `failed` 的 worker 才显示入口。

## Alternatives Considered

### Allow force drain for every NotReady node

未采用。K3s agent 重启、短暂网络故障和升级都会产生短时 `NotReady`，此时绕过 PDB 可能造成不必要的业务中断。

### Call `kubectl drain --disable-eviction`

未采用。平台已使用 client-go；直接调用 Pod Delete API 能保留结构化逐 Pod 结果，避免执行外部 CLI，也便于测试。

### Automatically remove the Node after force drain

未采用。删除 Node 会影响排障与存储恢复。强制驱逐仅执行故障转移，运维人员需在结果确认后显式选择现有“移出”操作。

## Risks And Mitigations

| 风险 | 缓解措施 |
| --- | --- |
| 错误判定宕机导致绕过 PDB | 仅 `Ready=Unknown` 且心跳超 5 分钟；必须输入节点名确认 |
| 误删不受控制器管理的 Pod | 将其阻止并列出，不提供绕过开关 |
| StatefulSet 数据不可立即可用 | 保留 Pod/卷类型与删除结果提示，不宣称已迁移完成 |
| control-plane 丢失影响仲裁 | 拒绝 control-plane 强制驱逐 |
| 普通驱逐排障信息不足 | 展示 pending Pod 的原始 API 错误 |

