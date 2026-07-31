# Tasks

## 1. Kubernetes node state and force drain

- [ ] 1.1 先为 Node Ready Condition 的健康状态派生编写单元测试，覆盖 ready、短暂 not_ready 和心跳超时 failed。
  - 验证：`go test ./internal/k8s -run TestNodeHealthState`
- [ ] 1.2 扩展 `NodeInfo` 与节点列表映射，返回健康状态、原因和最后心跳时间。
  - 验证：K8s fake-client 测试覆盖字段序列化。
- [ ] 1.3 先为故障 worker 强制驱逐编写测试，覆盖 cordon、直接删除受控 Pod、绕过 Eviction API、跳过 DaemonSet/static Pod、阻止无控制器 Pod。
  - 验证：`go test ./internal/k8s -run TestForceDrain`
- [ ] 1.4 实现故障节点强制驱逐及控制面、状态、确认参数的安全校验。
  - 验证：删除动作仅发生在满足全部前置条件时。
- [ ] 1.5 修正普通驱逐错误分类，保留逐 Pod 原始错误和结果。
  - 验证：测试覆盖 PDB 429 与非 PDB 429。

## 2. API and console

- [ ] 2.1 增加 `POST /api/nodes/:id/force-drain`，校验风险确认和节点名，并添加 handler 测试。
  - 验证：`go test ./internal/api -run TestNodeHandler_ForceDrain`
- [ ] 2.2 更新集群节点页面，展示健康状态和普通驱逐逐 Pod 结果。
  - 验证：Vue 测试覆盖 pending 原始原因展示。
- [ ] 2.3 实现故障 worker 强制驱逐对话框，包含影响清单、风险确认、节点名输入和最终结果。
  - 验证：Vue 测试覆盖入口可见性、确认拦截和提交请求。

## 3. Verification

- [ ] 3.1 执行 `go test ./...`、`go build ./...`、`npm test -- --run`、`npm run build`。
- [ ] 3.2 执行 `openspec validate failed-node-force-drain --strict`。
- [ ] 3.3 在真实集群使用已失联的 worker 验证：安全驱逐保留 PDB、强制驱逐只删除受控 Pod、Node 移出仍需人工确认。

