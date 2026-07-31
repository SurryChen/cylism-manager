# Failed Node Force Drain

## Why

现有节点驱逐只使用 Kubernetes Eviction API，严格遵守 PodDisruptionBudget (PDB)。当 worker 节点已经宕机时，源节点上的 Pod 不可能恢复，继续等待 PDB 放行会阻碍替代副本在健康节点创建。当前页面还将所有 Eviction API 的 HTTP 429 一律描述为 PDB 阻塞，且丢弃逐 Pod 的原始失败原因，无法支持准确排障。

## What Changes

- 依据 Kubernetes Node Ready Condition 与心跳时间识别故障 worker 节点，并在节点列表明确展示健康、未就绪和故障状态。
- 新增故障节点强制驱逐 API，只对故障 worker 节点开放。它 cordon 节点后直接删除可由控制器重建的 Pod，绕过 PDB。
- 强制驱逐拒绝无控制器管理的 Pod；DaemonSet 和 static/mirror Pod 不处理，并逐项说明原因。
- 强制操作要求调用方确认风险并精确提交目标节点名；前端要求勾选风险确认并输入节点名。
- 普通驱逐保留 Eviction API 与 PDB 保护；页面展示每个 pending Pod 的真实错误，而非统一宣称为 PDB。
- 强制驱逐完成后保留现有“移出节点”流程，不在强制驱逐中自动删除 Node 对象。
- 终端连接改用成熟 WebSocket 实现，支持长文本粘贴；终端选择文本不会关闭窗口，并统一使用不透明白色画布。

## Non-goals

- 不支持 Pod 的实时迁移、内存迁移或跨节点 checkpoint。
- 不允许对 control-plane 节点执行强制驱逐或强制移出。
- 不自动删除无控制器管理的 Pod，也不绕过 PVC 的存储附件和数据一致性约束。
- 不把 Node 的短暂 `Ready=False` 状态自动认定为机器故障。

## Impact

- `internal/k8s/node.go`: 节点故障状态、强制删除和驱逐结果分类。
- `internal/api/websocket.go`、终端组件：可靠 WebSocket 帧处理与终端交互修复。
- `internal/api/node_handler.go`、`router.go`: 强制驱逐 API 与请求校验。
- `web/src/views/Cluster.vue`: 故障状态展示、普通驱逐结果、强制驱逐确认对话框。
- `internal/k8s/node_test.go`、`internal/api/node_handler_test.go`、前端页面测试：覆盖安全边界和结果展示。
